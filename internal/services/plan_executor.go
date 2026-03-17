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
	llmClient       *LLMClient // Оставляем как fallback
}

type PlanStep struct {
	AgentType string
	Action    string
	Input     map[string]interface{}
	Output    map[string]interface{}
}

func NewPlanExecutor(dc discovery.Client, ac *AgentClient, llm *LLMClient) *PlanExecutor {
	return &PlanExecutor{
		discoveryClient: dc,
		agentClient:     ac,
		llmClient:       llm,
	}
}

// ParsePlan разбирает текст плана от LLM в структурированные шаги
func (e *PlanExecutor) ParsePlan(planText string) ([]PlanStep, error) {
	var steps []PlanStep

	// Очищаем текст от лишних символов
	planText = strings.TrimSpace(planText)

	// Разбиваем на строки
	lines := strings.Split(planText, "\n")

	// Улучшенное регулярное выражение - более гибкое
	stepRegex := regexp.MustCompile(`(?i)^\s*(\d+)[.)]\s*(\w+)\s*[→-]>?\s*(\w+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := stepRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			steps = append(steps, PlanStep{
				AgentType: strings.ToLower(matches[2]), // нормализуем в нижний регистр
				Action:    strings.ToLower(matches[3]),
				Input:     make(map[string]interface{}),
			})
			fmt.Printf("Parsed step: %s → %s\n", matches[2], matches[3])
		} else {
			// Логируем непонятные строки для отладки
			fmt.Printf("Skipping unparseable line: %s\n", line)
		}
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("no valid steps found in plan")
	}

	return steps, nil
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
		stepLog := fmt.Sprintf("Executing step %d: %s → %s", i+1, step.AgentType, step.Action)
		fmt.Println(stepLog)
		context["execution_log"] = append(context["execution_log"].([]string), stepLog)

		// Находим агента по типу
		agent, err := e.findAgentByType(step.AgentType)
		if err != nil {
			errorLog := fmt.Sprintf("Step %d failed: %v", i+1, err)
			context["execution_log"] = append(context["execution_log"].([]string), errorLog)
			continue
		}

		// Вызываем агента
		result, err := e.callAgent(agent, step, context)
		if err != nil {
			errorLog := fmt.Sprintf("Step %d failed: %v", i+1, err)
			context["execution_log"] = append(context["execution_log"].([]string), errorLog)
			continue
		}

		// Сохраняем результат
		resultKey := fmt.Sprintf("%s_result", step.AgentType)
		stepResults[resultKey] = result
		stepResults[fmt.Sprintf("step_%d_result", i+1)] = result

		successLog := fmt.Sprintf("Step %d completed successfully", i+1)
		context["execution_log"] = append(context["execution_log"].([]string), successLog)

		// Если это генератор, сохраняем финальный ответ отдельно
		if step.AgentType == "generator" {
			if response, ok := result["response"].(string); ok {
				context["final_response"] = response
			} else if text, ok := result["text"].(string); ok {
				context["final_response"] = text
			} else if response, ok := result["generated_text"].(string); ok {
				context["final_response"] = response
			}
		}
	}

	return context, nil
}

// УНИВЕРСАЛЬНЫЙ callAgent - теперь всё через agentClient
func (e *PlanExecutor) callAgent(agent *discovery.Agent, step PlanStep, context map[string]interface{}) (map[string]interface{}, error) {
	text, _ := context["original_text"].(string)
	config, _ := context["config"].(map[string]interface{})

	switch step.AgentType {
	case "classifier":
		// Специализированный вызов для классификатора
		payload := map[string]interface{}{
			"text": text,
		}
		return e.agentClient.call(agent.Endpoint+"/classify", payload)

	case "encoder":
		// Специализированный вызов для энкодера
		payload := map[string]interface{}{
			"texts": []string{text},
		}
		return e.agentClient.call(agent.Endpoint+"/embed", payload)

	case "researcher":
		// Заглушка для researcher
		return map[string]interface{}{
			"results": []map[string]interface{}{
				{"title": "Решение 1", "content": "Перезагрузите роутер", "relevance": 0.95},
				{"title": "Решение 2", "content": "Проверьте кабели", "relevance": 0.85},
			},
			"source": "knowledge_base",
		}, nil

	case "generator":
		fmt.Printf("🎯 Generator agent endpoint: %s\n", agent.Endpoint)
		fmt.Printf("🎯 Generator agent name: %s\n", agent.Name)

		// Собираем контекст из предыдущих шагов
		stepResults := context["step_results"].(map[string]interface{})

		// Получаем результаты классификации
		var classification map[string]interface{}
		var category string
		if cls, ok := stepResults["classifier_result"]; ok {
			classification = cls.(map[string]interface{})
			// Пробуем получить категорию из разных полей
			if cat, ok := classification["predicted_class"].(string); ok {
				category = cat
			} else if cat, ok := classification["category"].(string); ok {
				category = cat
			}
		}

		// Получаем результаты исследования
		solutions := make([]map[string]interface{}, 0)

		// Получаем результаты исследования, если они есть
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

		// Определяем стиль
		style := "friendly"
		if s, ok := config["communication_style"].(string); ok {
			style = s
		}

		// Формируем промпт
		prompt := e.buildGeneratorPrompt(text, classification, solutions, config)

		// 👇 ПРАВИЛЬНЫЙ payload для генератора
		payload := map[string]interface{}{
			"query":     text,
			"category":  category,
			"solutions": solutions,
			"style":     style,
			"context":   prompt, // Можно отправить сформированный промпт как контекст
			"language":  "ru",
		}

		fmt.Printf("📦 Sending payload to generator: %+v\n", payload)

		// Пробуем вызвать генератор по его эндпоинту
		var lastErr error
		result, err := e.agentClient.call(agent.Endpoint+"/v1/generate", payload)
		if err == nil {
			fmt.Printf("✅ Generator called successfully at %s\n", agent.Endpoint)
			return result, nil
		}
		lastErr = err
		fmt.Printf("❌ Failed at %s: %v\n", agent.Endpoint, err)

		// Если все попытки провалились, используем fallback
		fmt.Printf("All generator endpoints failed, using LLM fallback: %v\n", lastErr)
		return e.fallbackGenerator(prompt)

	default:
		return nil, fmt.Errorf("unsupported agent type: %s", step.AgentType)
	}
}

// buildGeneratorPrompt формирует промпт для генератора
func (e *PlanExecutor) buildGeneratorPrompt(text string, classification map[string]interface{}, solutions []map[string]interface{}, config map[string]interface{}) string {
	var builder strings.Builder

	builder.WriteString("Ты — агент поддержки, который помогает пользователям.\n\n")

	// Добавляем контекст компании
	if companyContext, ok := config["company_context"].(string); ok && companyContext != "" {
		builder.WriteString(fmt.Sprintf("Контекст компании: %s\n\n", companyContext))
	}

	// Добавляем обращение пользователя
	builder.WriteString(fmt.Sprintf("Обращение пользователя: \"%s\"\n\n", text))

	// Добавляем результаты классификации
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
		if entities, ok := classification["entities"].([]interface{}); ok && len(entities) > 0 {
			builder.WriteString(fmt.Sprintf("- Сущности: %v\n", entities))
		}
		builder.WriteString("\n")
	}

	// Добавляем результаты поиска
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

	// Инструкция по генерации ответа
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

	// Абсолютный fallback
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

	// Логируем всех найденных агентов
	for _, a := range agents {
		fmt.Printf("📋 Found agent: %s (ID: %s, Endpoint: %s, Caps: %v)\n",
			a.Name, a.ID, a.Endpoint, a.Capabilities)
	}

	// Выбираем подходящего агента по имени или ID
	var selectedAgent *discovery.Agent

	for i, a := range agents {
		// Для генератора выбираем по имени
		if agentType == "generator" && (strings.Contains(a.Name, "generator") || strings.Contains(a.Name, "llm")) {
			selectedAgent = &agents[i]
			break
		}
		// Для классификатора
		if agentType == "classifier" && strings.Contains(a.Name, "classifier") {
			selectedAgent = &agents[i]
			break
		}
		// Для энкодера
		if agentType == "encoder" && strings.Contains(a.Name, "encoder") {
			selectedAgent = &agents[i]
			break
		}
	}

	// Если не нашли по имени, берём первого
	if selectedAgent == nil {
		selectedAgent = &agents[0]
		fmt.Printf("⚠️ No specific agent found for type %s, using first: %s\n",
			agentType, selectedAgent.Name)
	}

	fmt.Printf("✅ Selected agent for %s: %s at %s\n",
		agentType, selectedAgent.Name, selectedAgent.Endpoint)

	return selectedAgent, nil
}
