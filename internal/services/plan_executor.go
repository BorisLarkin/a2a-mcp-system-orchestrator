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
}

type PlanStep struct {
	AgentType string
	Action    string
	Input     map[string]interface{}
	Output    map[string]interface{}
}

func NewPlanExecutor(dc discovery.Client, ac *AgentClient) *PlanExecutor {
	return &PlanExecutor{
		discoveryClient: dc,
		agentClient:     ac,
	}
}

// ParsePlan разбирает текст плана от LLM в структурированные шаги
func (e *PlanExecutor) ParsePlan(planText string) ([]PlanStep, error) {
	var steps []PlanStep

	// Простой парсинг: ищем строки вида "X. agent → action"
	lines := strings.Split(planText, "\n")
	stepRegex := regexp.MustCompile(`^\d+\.\s*(\w+)\s*→\s*(\w+)`)

	for _, line := range lines {
		matches := stepRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			steps = append(steps, PlanStep{
				AgentType: matches[1],
				Action:    matches[2],
				Input:     make(map[string]interface{}),
			})
		}
	}

	return steps, nil
}

// ExecutePlan выполняет последовательность шагов
// internal/services/plan_executor.go - обновлённый ExecutePlan

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
			continue // Продолжаем со следующим шагом вместо полного провала
		}

		// Вызываем агента
		result, err := e.callAgent(agent, step, context)
		if err != nil {
			errorLog := fmt.Sprintf("Step %d failed: %v", i+1, err)
			context["execution_log"] = append(context["execution_log"].([]string), errorLog)
			continue
		}

		// Сохраняем результат с понятным ключом
		resultKey := fmt.Sprintf("%s_result", step.AgentType)
		stepResults[resultKey] = result

		// Также сохраняем с индексом для истории
		stepResults[fmt.Sprintf("step_%d_result", i+1)] = result

		successLog := fmt.Sprintf("Step %d completed successfully", i+1)
		context["execution_log"] = append(context["execution_log"].([]string), successLog)

		// Если это генератор, сохраняем финальный ответ отдельно
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

// Обновим callAgent для правильной обработки ответов
func (e *PlanExecutor) callAgent(agent *discovery.Agent, step PlanStep, context map[string]interface{}) (map[string]interface{}, error) {
	text, _ := context["original_text"].(string)

	switch step.AgentType {
	case "classifier":
		return e.agentClient.CallClassification(agent.Endpoint, text)
	case "encoder":
		return e.agentClient.CallEmbedding(agent.Endpoint, text)
	case "researcher":
		// Пока заглушка, потом реализуем
		return map[string]interface{}{
			"results": []map[string]interface{}{
				{"title": "Решение 1", "content": "Перезагрузите роутер", "relevance": 0.95},
			},
			"source": "knowledge_base",
		}, nil
	case "generator":
		// Собираем контекст из предыдущих шагов
		stepResults := context["step_results"].(map[string]interface{})
		var classification map[string]interface{}
		if cls, ok := stepResults["classifier_result"]; ok {
			classification = cls.(map[string]interface{})
		}

		var researchResults map[string]interface{}
		if res, ok := stepResults["researcher_result"]; ok {
			researchResults = res.(map[string]interface{})
		}

		// Формируем промпт для генератора
		prompt := fmt.Sprintf("Обращение: %s\n", text)
		if classification != nil {
			prompt += fmt.Sprintf("Категория: %v\n", classification["predicted_class"])
		}
		if researchResults != nil {
			prompt += fmt.Sprintf("Найденные решения: %v\n", researchResults)
		}

		// TODO: реализовать вызов генератора
		return map[string]interface{}{
			"response": fmt.Sprintf("Спасибо за обращение. Мы получили ваш запрос и передали его в отдел поддержки."),
			"text":     fmt.Sprintf("Спасибо за обращение. Мы получили ваш запрос и передали его в отдел поддержки."),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported agent type: %s", step.AgentType)
	}
}

func (e *PlanExecutor) findAgentByType(agentType string) (*discovery.Agent, error) {
	// Маппинг типов агентов на capabilities
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
	if err != nil || len(agents) == 0 {
		return nil, fmt.Errorf("no agent found for capability: %s", capability)
	}

	return &agents[0], nil
}
