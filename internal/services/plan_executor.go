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
}

func NewPlanExecutor(dc discovery.Client, ac *AgentClient, a2a *A2AClient, llm *LLMClient) *PlanExecutor {
	return &PlanExecutor{
		discoveryClient: dc,
		agentClient:     ac,
		a2aClient:       a2a,
		llmClient:       llm,
	}
}

// ParsePlan разбирает текст плана от LLM
func (e *PlanExecutor) ParsePlan(planText string) ([]PlanStep, error) {
	var steps []PlanStep
	planText = strings.TrimSpace(planText)
	lines := strings.Split(planText, "\n")

	stepRegexWithSkill := regexp.MustCompile(`(?i)^\s*(\d+)[.)]\s*(\w+):(\w+)\s*[→-]>?\s*(.+)`)
	stepRegexSimple := regexp.MustCompile(`(?i)^\s*(\d+)[.)]\s*(\w+)\s*[→-]>?\s*(.+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := stepRegexWithSkill.FindStringSubmatch(line)
		if len(matches) == 5 {
			steps = append(steps, PlanStep{
				AgentType: strings.ToLower(matches[2]),
				SkillID:   strings.ToLower(matches[3]),
				Action:    strings.TrimSpace(matches[4]),
			})
			fmt.Printf("Parsed step (with skill): %s:%s → %s\n", matches[2], matches[3], strings.TrimSpace(matches[4]))
			continue
		}

		matches = stepRegexSimple.FindStringSubmatch(line)
		if len(matches) == 4 {
			agentType := strings.ToLower(matches[2])
			action := strings.TrimSpace(matches[3])
			skillID := getDefaultSkillID(agentType, action)
			steps = append(steps, PlanStep{
				AgentType: agentType,
				SkillID:   skillID,
				Action:    action,
			})
			fmt.Printf("Parsed step (simple): %s → %s (skill: %s)\n", agentType, action, skillID)
		} else {
			fmt.Printf("Skipping unparseable line: %s\n", line)
		}
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("no valid steps found in plan")
	}

	return steps, nil
}

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
	// Контекст выполнения — накапливает результаты всех шагов
	context := map[string]interface{}{
		"original_text": initialText,
		"text":          initialText,
		"query":         initialText,
		"config":        config,
		"step_results":  make(map[string]interface{}),
		"all_outputs":   []map[string]interface{}{}, // все output'ы для передачи агентам
	}

	stepResults := context["step_results"].(map[string]interface{})
	allOutputs := context["all_outputs"].([]map[string]interface{})

	for i, step := range steps {
		stepLog := fmt.Sprintf("Executing step %d: %s:%s → %s", i+1, step.AgentType, step.SkillID, step.Action)
		fmt.Println(stepLog)

		agent, err := e.findAgentByType(step.AgentType)
		if err != nil {
			fmt.Printf("Step %d failed: %v\n", i+1, err)
			continue
		}

		result, err := e.callAgent(agent, step, context)
		if err != nil {
			fmt.Printf("Step %d failed: %v\n", i+1, err)
			continue
		}

		// Сохраняем результат
		resultKey := fmt.Sprintf("%s_result", step.AgentType)
		stepResults[resultKey] = result
		stepResults[fmt.Sprintf("step_%d_result", i+1)] = result
		allOutputs = append(allOutputs, result)
		context["all_outputs"] = allOutputs

		// Если результат содержит поле "response" — это финальный ответ
		if response, ok := result["response"].(string); ok && response != "" {
			context["final_response"] = response
		}
	}

	context["step_results"] = stepResults
	return context, nil
}

// callAgent — универсальный вызов агента через A2A
// Собирает payload на основе input_schema агента и контекста выполнения
func (e *PlanExecutor) callAgent(agent *discovery.Agent, step PlanStep, context map[string]interface{}) (map[string]interface{}, error) {
	skill := agent.GetSkillByID(step.SkillID)
	if skill == nil {
		return nil, fmt.Errorf("skill %s not found for agent %s", step.SkillID, agent.Name)
	}

	// Собираем payload из контекста на основе input_schema
	payload := e.buildPayload(skill, context)

	fmt.Printf("A2A → Calling %s:%s at %s, payload keys: %v\n",
		step.AgentType, step.SkillID, agent.Endpoint, getKeys(payload))

	a2aResp, err := e.a2aClient.SendTask(agent.Endpoint, step.SkillID, payload)
	if err != nil {
		fmt.Printf("❌ A2A call failed for %s: %v, trying fallback...\n", step.AgentType, err)
		return e.fallbackCall(agent, step, context)
	}

	fmt.Printf("✅ A2A %s response: task_id=%s, status=%s\n",
		step.AgentType, a2aResp.TaskID, a2aResp.Status)
	return a2aResp.Output, nil
}

// buildPayload формирует payload для агента на основе его input_schema и контекста
func (e *PlanExecutor) buildPayload(skill *discovery.Skill, context map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{})

	// Всегда добавляем базовые поля
	if text, ok := context["original_text"].(string); ok {
		payload["text"] = text
		payload["query"] = text
	}

	// Добавляем конфигурацию
	if config, ok := context["config"].(map[string]interface{}); ok {
		payload["config"] = config
	}

	// Добавляем результаты предыдущих шагов (полные, нефильтрованные)
	if stepResults, ok := context["step_results"].(map[string]interface{}); ok && len(stepResults) > 0 {
		payload["previous_results"] = stepResults
	}

	// Матчим поля из input_schema с данными из контекста
	inputFields := skill.AllInputFields()
	stepResults, _ := context["step_results"].(map[string]interface{})

	for _, fieldName := range inputFields {
		// Пропускаем уже добавленные базовые поля
		if fieldName == "text" || fieldName == "query" || fieldName == "config" || fieldName == "previous_results" {
			continue
		}

		// Ищем поле среди результатов предыдущих шагов
		if val := extractFieldFromResults(stepResults, fieldName); val != nil {
			payload[fieldName] = val
		}
	}

	return payload
}

// extractFieldFromResults ищет значение поля в результатах всех предыдущих шагов
func extractFieldFromResults(stepResults map[string]interface{}, fieldName string) interface{} {
	for _, result := range stepResults {
		if resultMap, ok := result.(map[string]interface{}); ok {
			if val, exists := resultMap[fieldName]; exists {
				return val
			}
		}
	}
	return nil
}

// fallbackCall — заглушка на случай недоступности агента
func (e *PlanExecutor) fallbackCall(agent *discovery.Agent, step PlanStep, context map[string]interface{}) (map[string]interface{}, error) {
	fmt.Printf("⚠️ Using fallback for %s:%s\n", step.AgentType, step.SkillID)

	// Для генератора — используем локальную LLM
	if step.AgentType == "generator" {
		text, _ := context["original_text"].(string)
		if e.llmClient != nil {
			response, err := e.llmClient.Generate(
				"Ты — агент поддержки. Отвечай кратко и по делу.",
				fmt.Sprintf("Обращение: %s\n\nСгенерируй полезный ответ.", text),
			)
			if err == nil {
				return map[string]interface{}{
					"response": response,
					"source":   "llm_fallback",
				}, nil
			}
		}
		return map[string]interface{}{
			"response": "Спасибо за обращение. Мы получили ваш запрос и передали его в отдел поддержки.",
			"source":   "hardcoded_fallback",
		}, nil
	}

	// Для classifier — возвращаем базовую классификацию
	if step.AgentType == "classifier" {
		return map[string]interface{}{
			"category":        "общий_вопрос",
			"confidence":      0.3,
			"predicted_class": "общий_вопрос",
		}, nil
	}

	// Для researcher — пустой результат
	if step.AgentType == "researcher" {
		return map[string]interface{}{
			"results": []map[string]interface{}{},
			"source":  "fallback",
		}, nil
	}

	return map[string]interface{}{
		"error": fmt.Sprintf("agent %s unavailable", agent.Name),
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

	agents, err := e.discoveryClient.GetAgents("", []string{capability})
	if err != nil {
		return nil, fmt.Errorf("error getting agents: %w", err)
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agent found for capability: %s", capability)
	}

	// Выбираем подходящего агента по имени
	var selected *discovery.Agent
	for i, a := range agents {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(agentType)) {
			selected = &agents[i]
			break
		}
	}
	if selected == nil {
		selected = &agents[0]
	}

	fmt.Printf("✅ Selected agent for %s: %s at %s\n", agentType, selected.Name, selected.Endpoint)
	return selected, nil
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
