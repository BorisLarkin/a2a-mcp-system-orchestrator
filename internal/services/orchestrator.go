package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"orchestrator/internal/db"
	"orchestrator/internal/discovery"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OrchestratorService struct {
	discoveryClient discovery.Client
	agentClient     *AgentClient
	dispatcherRepo  *DispatcherRepository
	llmClient       *LLMClient
	planExecutor    *PlanExecutor
}

type ProcessTicketRequest struct {
	Text         string
	DispatcherID string
	Metadata     map[string]interface{}
}

type ProcessTicketResponse struct {
	TicketID       string                 `json:"ticket_id"`
	Classification map[string]interface{} `json:"classification,omitempty"`
	Embedding      []float64              `json:"embedding,omitempty"`
	SuggestedTeam  string                 `json:"suggested_team"`
	Status         string                 `json:"status"`
	Timestamp      string                 `json:"timestamp"`
	Plan           string                 `json:"plan,omitempty"`
	ExecutionLog   []string               `json:"execution_log,omitempty"`
}

func NewOrchestratorService(dc discovery.Client, ac *AgentClient, dr *DispatcherRepository, llm *LLMClient) *OrchestratorService {
	return &OrchestratorService{
		discoveryClient: dc,
		agentClient:     ac,
		dispatcherRepo:  dr,
		llmClient:       llm,
		planExecutor:    NewPlanExecutor(dc, ac),
	}
}

func (s *OrchestratorService) Process(ctx context.Context, req *ProcessTicketRequest) (*ProcessTicketResponse, error) {
	executionLog := []string{}
	stepResults := make(map[string]interface{}) // Добавляем для хранения результатов шагов

	// 1. Получаем конфигурацию диспетчерской
	dispatcher, err := s.dispatcherRepo.GetByID(req.DispatcherID)
	if err != nil {
		return nil, fmt.Errorf("dispatcher not found: %w", err)
	}

	logMsg := fmt.Sprintf("[Dispatcher: %s] Processing ticket from: %s", dispatcher.ID, dispatcher.Name)
	log.Println(logMsg)
	executionLog = append(executionLog, logMsg)

	// Парсим конфигурацию
	var configMap map[string]interface{}
	if err := json.Unmarshal(dispatcher.Config, &configMap); err != nil {
		configMap = make(map[string]interface{})
		log.Printf("[Warning] Could not parse dispatcher config: %v", err)
	}

	// 2. Получаем список всех доступных агентов
	allAgents, err := s.discoveryClient.GetAgents(req.DispatcherID, []string{})
	if err != nil {
		log.Printf("Warning: failed to get agents list: %v", err)
		executionLog = append(executionLog, fmt.Sprintf("Warning: failed to get agents list: %v", err))
	}

	// Логируем доступных агентов
	agentList := []string{}
	for _, a := range allAgents {
		agentList = append(agentList, fmt.Sprintf("%s (%s)", a.Name, strings.Join(a.Capabilities, ",")))
	}
	log.Printf("Available agents: %s", strings.Join(agentList, ", "))
	executionLog = append(executionLog, fmt.Sprintf("Available agents: %s", strings.Join(agentList, ", ")))

	// 3. LLM планирует обработку
	log.Println("🤔 Requesting plan from LLM...")
	plan, err := s.createPlan(req.Text, configMap, allAgents)
	if err != nil {
		log.Printf("LLM planning failed, falling back to default plan: %v", err)
		plan = s.defaultPlan()
		executionLog = append(executionLog, fmt.Sprintf("LLM planning failed: %v, using default plan", err))
	} else {
		log.Printf("[LLM Plan] %s", plan)
		executionLog = append(executionLog, fmt.Sprintf("LLM Plan: %s", plan))
	}

	// 4. Парсим и выполняем план
	steps, parseErr := s.planExecutor.ParsePlan(plan)
	if parseErr != nil {
		log.Printf("Failed to parse plan: %v", parseErr)
		executionLog = append(executionLog, fmt.Sprintf("Failed to parse plan: %v", parseErr))
		// Всё равно пытаемся выполнить базовую обработку
		return s.fallbackProcess(req, dispatcher, configMap, executionLog)
	}

	if len(steps) == 0 {
		log.Println("No steps found in plan, using fallback")
		executionLog = append(executionLog, "No steps found in plan, using fallback")
		return s.fallbackProcess(req, dispatcher, configMap, executionLog)
	}

	// Выполняем план
	log.Printf("Executing plan with %d steps", len(steps))
	executionLog = append(executionLog, fmt.Sprintf("Executing plan with %d steps", len(steps)))

	executionContext, execErr := s.planExecutor.ExecutePlan(steps, req.Text, configMap)
	if execErr != nil {
		log.Printf("Plan execution failed: %v", execErr)
		executionLog = append(executionLog, fmt.Sprintf("Plan execution failed: %v", execErr))
		return s.fallbackProcess(req, dispatcher, configMap, executionLog)
	}

	// Сохраняем результаты шагов для ответа
	if ctx, ok := executionContext["step_results"].(map[string]interface{}); ok {
		stepResults = ctx
	}

	// Логируем каждый результат
	for stepName, result := range stepResults {
		resultJSON, _ := json.Marshal(result)
		log.Printf("📊 Step '%s' result: %s", stepName, string(resultJSON))
		executionLog = append(executionLog, fmt.Sprintf("Step '%s' result: %v", stepName, result))
	}

	// Извлекаем результаты с правильной обработкой confidence
	var classification map[string]interface{}
	var embedding []float64
	var finalResponse string
	var predictedClass string
	var confidence float64

	// Получаем classification из stepResults
	if cls, ok := stepResults["classifier_result"].(map[string]interface{}); ok {
		classification = cls

		// Пробуем разные возможные поля для класса
		if pc, ok := cls["predicted_class"].(string); ok {
			predictedClass = pc
		} else if pc, ok := cls["category"].(string); ok {
			predictedClass = pc
			// Нормализуем поле
			cls["predicted_class"] = pc
		}

		// Пробуем разные возможные поля для confidence
		if conf, ok := cls["confidence"].(float64); ok {
			confidence = conf
		} else if conf, ok := cls["score"].(float64); ok {
			confidence = conf
			cls["confidence"] = conf
		} else if scores, ok := cls["scores"].(map[string]interface{}); ok {
			// Если есть scores, берём максимальный
			var maxScore float64
			for _, v := range scores {
				if score, ok := v.(float64); ok && score > maxScore {
					maxScore = score
				}
			}
			if maxScore > 0 {
				confidence = maxScore
				cls["confidence"] = maxScore
			}
		}

		log.Printf("📊 Extracted - class: '%s', confidence: %.2f", predictedClass, confidence)
	}

	// Получаем embedding если есть
	if emb, ok := stepResults["embedding_result"].([]float64); ok {
		embedding = emb
	} else if embInterface, ok := stepResults["embedding_result"].(map[string]interface{}); ok {
		if embeddings, ok := embInterface["embeddings"].([]interface{}); ok && len(embeddings) > 0 {
			if first, ok := embeddings[0].([]interface{}); ok {
				embedding = make([]float64, len(first))
				for i, v := range first {
					if val, ok := v.(float64); ok {
						embedding[i] = val
					}
				}
			}
		}
	}

	// Получаем финальный ответ от генератора
	if resp, ok := stepResults["generator_response"].(string); ok {
		finalResponse = resp
	} else if genResult, ok := stepResults["generator_result"].(map[string]interface{}); ok {
		if resp, ok := genResult["response"].(string); ok {
			finalResponse = resp
		} else if text, ok := genResult["text"].(string); ok {
			finalResponse = text
		}
	}

	// Если не получили финальный ответ, но есть classification, генерируем простой
	if finalResponse == "" && predictedClass != "" {
		finalResponse = fmt.Sprintf("Ваш запрос категории '%s' принят в обработку.", predictedClass)
		if confidence > 0 {
			finalResponse += fmt.Sprintf(" (уверенность: %.0f%%)", confidence*100)
		}
	}

	// Порог уверенности
	threshold := 0.7
	if th, ok := configMap["confidence_threshold"].(float64); ok {
		threshold = th
	}

	// Принимаем решение на основе confidence
	if confidence < threshold {
		decisionMsg := fmt.Sprintf("[Decision] Confidence (%.2f) below threshold (%.2f) - escalating", confidence, threshold)
		log.Println(decisionMsg)
		executionLog = append(executionLog, decisionMsg)
		if classification != nil {
			classification["needs_human_review"] = true
		}
	} else {
		decisionMsg := fmt.Sprintf("[Decision] Confidence (%.2f) meets threshold (%.2f) - auto-responding", confidence, threshold)
		log.Println(decisionMsg)
		executionLog = append(executionLog, decisionMsg)
	}

	// Определяем команду
	team := s.resolveTeam(predictedClass, configMap)

	// Добавляем метаданные в classification
	if classification != nil {
		classification["threshold_met"] = confidence >= threshold
		classification["used_threshold"] = threshold
		if finalResponse != "" {
			classification["generated_response"] = finalResponse
		}
	}

	response := &ProcessTicketResponse{
		TicketID:       uuid.New().String(),
		Classification: classification,
		Embedding:      embedding,
		SuggestedTeam:  team,
		Status:         "processed",
		Timestamp:      time.Now().Format(time.RFC3339),
		Plan:           plan,
		ExecutionLog:   executionLog,
	}

	log.Printf("✅ Ticket processed successfully")
	return response, nil
}

// createPlan запрашивает план у LLM
func (s *OrchestratorService) createPlan(text string, config map[string]interface{}, agents []discovery.Agent) (string, error) {
	// Формируем список доступных агентов для промпта
	agentDescriptions := []string{}
	for _, a := range agents {
		agentDescriptions = append(agentDescriptions, fmt.Sprintf("- %s: capabilities: %v", a.Name, a.Capabilities))
	}
	agentsText := strings.Join(agentDescriptions, "\n")

	// Формируем системный промпт
	systemPrompt := `Ты — оркестратор системы поддержки. Твоя задача — спланировать обработку обращения пользователя.

Доступные агенты:
%s

Правила планирования:
1. Всегда начинай с классификации (classifier)
2. Если нужно найти решение в базе знаний — используй researcher
3. Для поиска похожих случаев нужны эмбеддинги (encoder)
4. Завершай генерацией ответа (generator)

Формат ответа:
Каждый шаг с новой строки в формате: "X. агент → действие"
Пример:
1. classifier → category
2. researcher → search
3. generator → response

Не добавляй лишнего текста, только план.`

	systemPrompt = fmt.Sprintf(systemPrompt, agentsText)

	userPrompt := fmt.Sprintf(`Конфигурация компании: %v
Обращение пользователя: "%s"

Составь план обработки из 2-3 шагов.`, config, text)

	return s.llmClient.Generate(systemPrompt, userPrompt)
}

// defaultPlan возвращает план по умолчанию
func (s *OrchestratorService) defaultPlan() string {
	return `1. classifier → category
2. encoder → embedding
3. generator → response`
}

// fallbackProcess используется когда план не сработал
func (s *OrchestratorService) fallbackProcess(req *ProcessTicketRequest, dispatcher *db.Dispatcher, configMap map[string]interface{}, executionLog []string) (*ProcessTicketResponse, error) {
	log.Println("Using fallback processing (direct agent calls)")
	executionLog = append(executionLog, "Using fallback processing (direct agent calls)")

	// Получаем всех агентов
	allAgents, _ := s.discoveryClient.GetAgents(req.DispatcherID, []string{})

	// Находим классификатор
	var classifierAgent *discovery.Agent
	var encoderAgent *discovery.Agent

	for _, a := range allAgents {
		for _, cap := range a.Capabilities {
			if cap == "classification" && classifierAgent == nil {
				classifierAgent = &a
			}
			if cap == "embedding" && encoderAgent == nil {
				encoderAgent = &a
			}
		}
	}

	var classification map[string]interface{}
	var embedding []float64
	var predictedClass string
	var confidence float64

	// Вызываем классификатор если есть
	if classifierAgent != nil {
		log.Printf("Calling classifier: %s", classifierAgent.Endpoint)
		executionLog = append(executionLog, fmt.Sprintf("Calling classifier: %s", classifierAgent.Endpoint))

		cls, err := s.agentClient.CallClassification(classifierAgent.Endpoint, req.Text)
		if err == nil {
			classification = cls
			if pc, ok := cls["predicted_class"].(string); ok {
				predictedClass = pc
			}
			if conf, ok := cls["confidence"].(float64); ok {
				confidence = conf
			}
			executionLog = append(executionLog, fmt.Sprintf("Classification result: %s (%.2f)", predictedClass, confidence))
		} else {
			log.Printf("Classification failed: %v", err)
			executionLog = append(executionLog, fmt.Sprintf("Classification failed: %v", err))
		}
	}

	// Вызываем энкодер если есть
	if encoderAgent != nil {
		log.Printf("Calling encoder: %s", encoderAgent.Endpoint)
		executionLog = append(executionLog, fmt.Sprintf("Calling encoder: %s", encoderAgent.Endpoint))

		emb, err := s.agentClient.CallEmbedding(encoderAgent.Endpoint, req.Text)
		if err == nil {
			if embeddings, ok := emb["embeddings"].([]interface{}); ok && len(embeddings) > 0 {
				if first, ok := embeddings[0].([]interface{}); ok {
					embedding = make([]float64, len(first))
					for i, v := range first {
						if val, ok := v.(float64); ok {
							embedding[i] = val
						}
					}
				}
			}
			executionLog = append(executionLog, "Embedding created successfully")
		} else {
			log.Printf("Embedding failed: %v", err)
			executionLog = append(executionLog, fmt.Sprintf("Embedding failed: %v", err))
		}
	}

	// Порог уверенности
	threshold := 0.7
	if th, ok := configMap["confidence_threshold"].(float64); ok {
		threshold = th
	}

	// Добавляем метаданные
	if classification != nil {
		classification["threshold_met"] = confidence >= threshold
		classification["used_threshold"] = threshold
	}

	// Определяем команду
	team := s.resolveTeam(predictedClass, configMap)

	return &ProcessTicketResponse{
		TicketID:       uuid.New().String(),
		Classification: classification,
		Embedding:      embedding,
		SuggestedTeam:  team,
		Status:         "processed",
		Timestamp:      time.Now().Format(time.RFC3339),
		Plan:           "fallback",
		ExecutionLog:   executionLog,
	}, nil
}

// resolveTeam определяет команду поддержки на основе класса обращения
func (s *OrchestratorService) resolveTeam(class string, config map[string]interface{}) string {
	teams := map[string]string{
		"техническая":   "tech_support",
		"биллинг":       "billing_team",
		"жалоба":        "complaint_department",
		"общий_вопрос":  "general_support",
		"срочное":       "urgent_team",
		"network_issue": "tech_support",
		"account":       "billing_team",
	}

	// Учитываем стиль общения из конфига
	if style, ok := config["communication_style"].(string); ok && style == "formal" {
		if team, exists := teams[class]; exists {
			return "official_" + team
		}
	}

	if team, exists := teams[class]; exists {
		return team
	}
	return "general_support"
}
