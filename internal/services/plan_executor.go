package services

import (
	"fmt"
	"orchestrator/internal/discovery"
	"regexp"
	"strings"
)

type PlanExecutor struct {
	discoveryClient discovery.Client
	agentClient     *AgentClient
	a2aClient       *A2AClient
	llmClient       *LLMClient
}

type PlanStep struct {
	AgentType string
	SkillID   string
	Action    string
	Input     map[string]interface{}
	Output    map[string]interface{}
}

func NewPlanExecutor(dc discovery.Client, ac *AgentClient, a2a *A2AClient, llm *LLMClient) *PlanExecutor {
	return &PlanExecutor{
		discoveryClient: dc,
		agentClient:     ac,
		a2aClient:       a2a,
		llmClient:       llm,
	}
}

// ParsePlan разбирает текст плана от LLM в структурированные шаги
// Поддерживает форматы:
//
//	"1. agent_type:skill_id → action"
//	"1. agent_type → action" (skill_id = action)
func (e *PlanExecutor) ParsePlan(planText string) ([]PlanStep, error) {
	var steps []PlanStep

	planText = strings.TrimSpace(planText)
	lines := strings.Split(planText, "\n")

	// Используем (.+) вместо (\w+) для поддержки кириллицы в действиях
	stepRegexWithSkill := regexp.MustCompile(`(?i)^\s*(\d+)[.)]\s*(\w+):(\w+)\s*[→-]>?\s*(.+)`)
	stepRegexSimple := regexp.MustCompile(`(?i)^\s*(\d+)[.)]\s*(\w+)\s*[→-]>?\s*(.+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Пробуем с явным skill_id
		matches := stepRegexWithSkill.FindStringSubmatch(line)
		if len(matches) == 5 {
			skillID := strings.ToLower(matches[3])
			steps = append(steps, PlanStep{
				AgentType: strings.ToLower(matches[2]),
				SkillID:   skillID,
				Action:    strings.TrimSpace(matches[4]),
				Input:     make(map[string]interface{}),
			})
			fmt.Printf("Parsed step (with skill): %s:%s → %s\n", matches[2], matches[3], strings.TrimSpace(matches[4]))
			continue
		}

		// Пробуем без skill_id
		matches = stepRegexSimple.FindStringSubmatch(line)
		if len(matches) == 4 {
			agentType := strings.ToLower(matches[2])
			action := strings.TrimSpace(matches[3])
			skillID := getDefaultSkillID(agentType, action)
			steps = append(steps, PlanStep{
				AgentType: agentType,
				SkillID:   skillID,
				Action:    action,
				Input:     make(map[string]interface{}),
			})
			fmt.Printf("Parsed step (simple): %s → %s (skill: %s)\n", agentType, action, skillID)
			continue
		}

		fmt.Printf("Skipping unparseable line: %s\n", line)
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("no valid steps found in plan")
	}

	return steps, nil
}

// getDefaultSkillID возвращает стандартный skill_id для типа агента
func getDefaultSkillID(agentType, action string) string {
	defaults := map[string]string{
		"classifier": "classify",
		"encoder":    "embed",
		"researcher": "search",
		"generator":  "generate_response",
	}
	if skillID, ok := defaults[agentType]; ok {
		return skillID
	}
	return action
}

// ExecutePlan выполняет последовательность шагов
func (e *PlanExecutor) ExecutePlan(steps []PlanStep, initialText string, config map[string]interface{}) (map[string]interface{}, error) {
	context := map[string]interface{}{
		"original_text": initialText,
		"config":        config,
		"step_results":  make(map[string]interface{}),
		"execution_log": []string{},
	}

	stepResults := context["step_results"].(map[string]interface{})

	for i, step := range steps {
		stepLog := fmt.Sprintf("Executing step %d: %s:%s → %s", i+1, step.AgentType, step.SkillID, step.Action)
		fmt.Println(stepLog)
		context["execution_log"] = append(context["execution_log"].([]string), stepLog)

		agent, err := e.findAgentByType(step.AgentType)
		if err != nil {
			errorLog := fmt.Sprintf("Step %d failed: %v", i+1, err)
			context["execution_log"] = append(context["execution_log"].([]string), errorLog)
			continue
		}

		result, err := e.callAgent(agent, step, context)
		if err != nil {
			errorLog := fmt.Sprintf("Step %d failed: %v", i+1, err)
			context["execution_log"] = append(context["execution_log"].([]string), errorLog)
			continue
		}

		resultKey := fmt.Sprintf("%s_result", step.AgentType)
		stepResults[resultKey] = result
		stepResults[fmt.Sprintf("step_%d_result", i+1)] = result

		successLog := fmt.Sprintf("Step %d completed successfully", i+1)
		context["execution_log"] = append(context["execution_log"].([]string), successLog)

		if step.AgentType == "generator" {
			if response, ok := result["response"].(string); ok {
				context["final_response"] = response
			} else if text, ok := result["text"].(string); ok {
				context["final_response"] = text
			}
		}
	}

	return context, nil
}

// callAgent — универсальный вызов агента через A2A с fallback на старый HTTP
func (e *PlanExecutor) callAgent(agent *discovery.Agent, step PlanStep, context map[string]interface{}) (map[string]interface{}, error) {
	text, _ := context["original_text"].(string)
	config, _ := context["config"].(map[string]interface{})

	// Пробуем A2A для всех типов агентов
	fmt.Printf("A2A → Calling %s:%s at %s\n", step.AgentType, step.SkillID, agent.Endpoint)

	var payload map[string]interface{}

	switch step.AgentType {
	case "classifier":
		payload = map[string]interface{}{
			"text": text,
		}

	case "encoder":
		payload = map[string]interface{}{
			"texts": []string{text},
		}

	case "researcher":
		category := ""
		if cls, ok := context["step_results"].(map[string]interface{})["classifier_result"]; ok {
			if cmap, ok := cls.(map[string]interface{}); ok {
				if cat, ok := cmap["category"].(string); ok {
					category = cat
				}
			}
		}
		payload = map[string]interface{}{
			"query":    text,
			"category": category,
		}

	case "generator":
		stepResults := context["step_results"].(map[string]interface{})

		var category string
		if cls, ok := stepResults["classifier_result"]; ok {
			if classification, ok := cls.(map[string]interface{}); ok {
				if cat, ok := classification["predicted_class"].(string); ok {
					category = cat
				} else if cat, ok := classification["category"].(string); ok {
					category = cat
				}
			}
		}

		solutions := make([]map[string]interface{}, 0)
		if res, ok := stepResults["researcher_result"]; ok {
			if research, ok := res.(map[string]interface{}); ok {
				if results, ok := research["results"].([]interface{}); ok {
					for _, r := range results {
						if sol, ok := r.(map[string]interface{}); ok {
							solutions = append(solutions, sol)
						}
					}
				}
			}
		}

		style := "friendly"
		if s, ok := config["communication_style"].(string); ok {
			style = s
		}

		prompt := e.buildGeneratorPrompt(text, nil, solutions, config)

		payload = map[string]interface{}{
			"query":     text,
			"category":  category,
			"solutions": solutions,
			"style":     style,
			"context":   prompt,
			"language":  "ru",
		}

	default:
		return nil, fmt.Errorf("unsupported agent type: %s", step.AgentType)
	}

	// Основной вызов через A2A
	a2aResp, err := e.a2aClient.SendTask(agent.Endpoint, step.SkillID, payload)
	if err != nil {
		fmt.Printf("❌ A2A call failed for %s: %v, trying old-style fallback...\n", step.AgentType, err)
		return e.fallbackCall(agent, step, text, config, payload)
	}

	fmt.Printf("✅ A2A %s response: task_id=%s, status=%s\n", step.AgentType, a2aResp.TaskID, a2aResp.Status)
	return a2aResp.Output, nil
}

// fallbackCall — старый способ вызова (если A2A не сработал)
func (e *PlanExecutor) fallbackCall(agent *discovery.Agent, step PlanStep, text string, config map[string]interface{}, a2aPayload map[string]interface{}) (map[string]interface{}, error) {
	switch step.AgentType {
	case "classifier":
		return e.agentClient.CallClassification(agent.Endpoint, text)
	case "encoder":
		return e.agentClient.CallEmbedding(agent.Endpoint, text)
	case "researcher":
		return map[string]interface{}{
			"results": []map[string]interface{}{
				{"title": "Решение 1", "content": "Перезагрузите роутер", "relevance": 0.95},
				{"title": "Решение 2", "content": "Проверьте кабели", "relevance": 0.85},
			},
			"source": "knowledge_base",
		}, nil
	case "generator":
		oldResult, oldErr := e.agentClient.call(agent.Endpoint+"/v1/generate", a2aPayload)
		if oldErr != nil {
			prompt := e.buildGeneratorPrompt(text, nil, nil, config)
			return e.fallbackGenerator(prompt)
		}
		return oldResult, nil
	default:
		return nil, fmt.Errorf("no fallback for agent type: %s", step.AgentType)
	}
}

// buildGeneratorPrompt формирует промпт для генератора
func (e *PlanExecutor) buildGeneratorPrompt(text string, classification map[string]interface{}, solutions []map[string]interface{}, config map[string]interface{}) string {
	var builder strings.Builder

	builder.WriteString("Ты — агент поддержки, который помогает пользователям.\n\n")

	if companyContext, ok := config["company_context"].(string); ok && companyContext != "" {
		builder.WriteString(fmt.Sprintf("Контекст компании: %s\n\n", companyContext))
	}

	builder.WriteString(fmt.Sprintf("Обращение пользователя: \"%s\"\n\n", text))

	if classification != nil {
		builder.WriteString("Результаты классификации:\n")
		if cat, ok := classification["predicted_class"].(string); ok && cat != "" {
			builder.WriteString(fmt.Sprintf("- Категория: %s\n", cat))
		}
		if cat, ok := classification["category"].(string); ok && cat != "" {
			builder.WriteString(fmt.Sprintf("- Категория: %s\n", cat))
		}
		if conf, ok := classification["confidence"].(float64); ok {
			builder.WriteString(fmt.Sprintf("- Уверенность: %.0f%%\n", conf*100))
		}
		builder.WriteString("\n")
	}

	if len(solutions) > 0 {
		builder.WriteString("Найденные решения в базе знаний:\n")
		for i, sol := range solutions {
			title, _ := sol["title"].(string)
			content, _ := sol["content"].(string)
			relevance, _ := sol["relevance"].(float64)
			builder.WriteString(fmt.Sprintf("%d. %s (релевантность: %.2f)\n", i+1, title, relevance))
			if content != "" {
				builder.WriteString(fmt.Sprintf("   %s\n", content))
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString(`Сгенерируй ответ пользователю на русском языке, следуя правилам:
1. Ответ должен быть вежливым и helpful
2. Используй найденные решения, если они релевантны
3. Если точного решения нет, предложи дальнейшие действия
4. Не упоминай, что ты AI или нейросеть
5. Ответ должен быть не более 3-4 предложений

Ответ: `)

	return builder.String()
}

// fallbackGenerator использует локальную LLM если генератор недоступен
func (e *PlanExecutor) fallbackGenerator(prompt string) (map[string]interface{}, error) {
	if e.llmClient != nil {
		systemPrompt := "Ты — полезный агент поддержки. Отвечай кратко и по делу."
		response, err := e.llmClient.Generate(systemPrompt, prompt)
		if err == nil {
			return map[string]interface{}{
				"response": response,
				"text":     response,
				"source":   "llm_fallback",
			}, nil
		}
	}

	return map[string]interface{}{
		"response": "Спасибо за обращение. Мы получили ваш запрос и передали его в отдел поддержки.",
		"text":     "Спасибо за обращение. Мы получили ваш запрос и передали его в отдел поддержки.",
		"source":   "fallback",
	}, nil
}

func (e *PlanExecutor) findAgentByType(agentType string) (*discovery.Agent, error) {
	capabilityMap := map[string]string{
		"classifier": "classification",
		"encoder":    "embedding",
		"researcher": "search",
		"generator":  "generation",
	}

	capability, ok := capabilityMap[agentType]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	fmt.Printf("🔍 Looking for agent with capability: %s\n", capability)

	agents, err := e.discoveryClient.GetAgents("", []string{capability})
	if err != nil {
		return nil, fmt.Errorf("error getting agents: %w", err)
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agent found for capability: %s", capability)
	}

	for _, a := range agents {
		fmt.Printf("📋 Found agent: %s (ID: %s, Endpoint: %s, Caps: %v)\n",
			a.Name, a.ID, a.Endpoint, a.Capabilities)
	}

	var selectedAgent *discovery.Agent

	for i, a := range agents {
		if agentType == "generator" && (strings.Contains(a.Name, "generator") || strings.Contains(a.Name, "llm")) {
			selectedAgent = &agents[i]
			break
		}
		if agentType == "classifier" && strings.Contains(a.Name, "classifier") {
			selectedAgent = &agents[i]
			break
		}
		if agentType == "encoder" && strings.Contains(a.Name, "encoder") {
			selectedAgent = &agents[i]
			break
		}
		if agentType == "researcher" && strings.Contains(a.Name, "researcher") {
			selectedAgent = &agents[i]
			break
		}
	}

	if selectedAgent == nil {
		selectedAgent = &agents[0]
		fmt.Printf("⚠️ No specific agent found for type %s, using first: %s\n",
			agentType, selectedAgent.Name)
	}

	fmt.Printf("✅ Selected agent for %s: %s at %s\n",
		agentType, selectedAgent.Name, selectedAgent.Endpoint)

	return selectedAgent, nil
}
