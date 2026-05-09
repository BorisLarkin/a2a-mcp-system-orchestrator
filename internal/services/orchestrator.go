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
	"gorm.io/datatypes"
)

type OrchestratorService struct {
	discoveryClient discovery.Client
	agentClient     *AgentClient
	a2aClient       *A2AClient
	dispatcherRepo  *DispatcherRepository
	agentRepo       *AgentRepository
	ticketRepo      *TicketRepository
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

func NewOrchestratorService(
	dc discovery.Client,
	ac *AgentClient,
	a2a *A2AClient,
	dr *DispatcherRepository,
	ar *AgentRepository,
	tr *TicketRepository,
	llm *LLMClient,
) *OrchestratorService {
	return &OrchestratorService{
		discoveryClient: dc,
		agentClient:     ac,
		a2aClient:       a2a,
		dispatcherRepo:  dr,
		agentRepo:       ar,
		ticketRepo:      tr,
		llmClient:       llm,
		planExecutor:    NewPlanExecutor(dc, ac, a2a, llm),
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

	// --- Schema-driven извлечение данных ---
	var classification map[string]interface{}
	var finalResponse string
	var confidence float64
	var predictedClass string

	// Ищем confidence в результатах всех шагов, используя Agent Card'ы
	for _, agent := range allAgents {
		agentType := agentToAgentType(agent)
		if result, ok := stepResults[agentType+"_result"]; ok {
			if resultMap, ok := result.(map[string]interface{}); ok {
				// Копируем результат классификатора как classification
				if agentType == "classifier" {
					classification = resultMap
				}

				// Извлекаем confidence по schema
				if cf, found := extractConfidence(&agent, resultMap); found {
					confidence = cf
				}

				// Извлекаем категорию/класс
				if pc, ok := resultMap["predicted_class"].(string); ok {
					predictedClass = pc
				} else if cat, ok := resultMap["category"].(string); ok {
					predictedClass = cat
				}
			}
		}
	}

	// Извлекаем финальный ответ из результатов генератора
	if resp, ok := stepResults["generator_result"].(map[string]interface{}); ok {
		if r, ok := resp["response"].(string); ok {
			finalResponse = r
		}
	}

	// Если не нашли ответ в generator_result, ищем в любом результате
	if finalResponse == "" {
		for _, result := range stepResults {
			if resultMap, ok := result.(map[string]interface{}); ok {
				if r, ok := resultMap["response"].(string); ok && r != "" {
					finalResponse = r
					break
				}
			}
		}
	}

	// Порог уверенности
	threshold := 0.7
	if th, ok := configMap["confidence_threshold"].(float64); ok {
		threshold = th
	}

	// Принимаем решение
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
		SuggestedTeam:  team,
		Status:         "processed",
		Timestamp:      time.Now().Format(time.RFC3339),
		Plan:           plan,
		ExecutionLog:   executionLog,
	}

	ticketID, _ := uuid.Parse(response.TicketID)
	dispID, _ := uuid.Parse(req.DispatcherID)
	ticket := &db.Ticket{
		ID:               ticketID,
		DispatcherID:     &dispID,
		Text:             req.Text,
		Status:           response.Status,
		Plan:             &response.Plan,
		Confidence:       &confidence,
		SuggestedTeam:    &team,
		ProcessingTimeMs: nil,
	}
	if classification != nil {
		classJSON, _ := json.Marshal(classification)
		ticket.Classification = datatypes.JSON(classJSON)
	}
	if finalResponse != "" {
		ticket.FinalResponse = &finalResponse
	}
	if len(executionLog) > 0 {
		logJSON, _ := json.Marshal(executionLog)
		ticket.ExecutionLog = datatypes.JSON(logJSON)
	}
	if err := s.ticketRepo.Create(ticket); err != nil {
		log.Printf("Warning: failed to save ticket: %v", err)
	} else {
		log.Printf("Ticket saved: %s", ticket.ID)
	}
	log.Printf("✅ Ticket processed successfully")
	return response, nil
}

// extractConfidence извлекает confidence из ответа агента на основе Agent Card
func extractConfidence(agent *discovery.Agent, output map[string]interface{}) (float64, bool) {
	for _, skill := range agent.Skills {
		// Ищем confidence_field в output_schema
		if cf, ok := skill.OutputSchema["confidence_field"].(string); ok {
			if val, ok := output[cf].(float64); ok {
				return val, true
			}
		}
		// Ищем поле "confidence" в properties output_schema
		if props, ok := skill.OutputSchema["properties"].(map[string]interface{}); ok {
			if _, hasConf := props["confidence"]; hasConf {
				if val, ok := output["confidence"].(float64); ok {
					return val, true
				}
			}
		}
		// Пробуем score, если confidence нет
		if val, ok := output["confidence"].(float64); ok {
			return val, true
		}
	}
	return 0, false
}

// agentToAgentType возвращает тип агента по его capabilities
func agentToAgentType(agent discovery.Agent) string {
	for _, cap := range agent.Capabilities {
		switch cap {
		case "classification":
			return "classifier"
		case "embedding":
			return "encoder"
		case "search":
			return "researcher"
		case "generation":
			return "generator"
		}
	}
	return strings.ToLower(agent.Name)
}

func (s *OrchestratorService) createPlan(text string, config map[string]interface{}, agents []discovery.Agent) (string, error) {
	agentDescriptions := []string{}
	for _, a := range agents {
		skills := []string{}
		for _, sk := range a.Skills {
			skills = append(skills, sk.ID)
		}
		agentDescriptions = append(agentDescriptions,
			fmt.Sprintf("- %s: capabilities: %v, skills: %v", a.Name, a.Capabilities, skills))
	}
	agentsText := strings.Join(agentDescriptions, "\n")

	systemPrompt := `Ты — оркестратор системы поддержки. Твоя задача — спланировать обработку обращения пользователя.

Доступные агенты:
%s

ВАЖНО: Твой ответ должен быть ТОЛЬКО списком шагов в формате:
"X. агент:навык → действие"

ПРИМЕРЫ ПРАВИЛЬНЫХ ОТВЕТОВ:
1. classifier:classify → problem_type
2. researcher:search → solutions
3. generator:generate_response → answer

1. classifier:classify → category
2. encoder:embed → embedding
3. generator:generate_response → response

ЗАПРЕЩЕНО:
- Использовать if/else
- Писать объяснения
- Добавлять скобки или специальные символы
- Менять формат "номер. агент:навык → действие"
- Писать агента без навыка (например "1. classifier → category" — НЕЛЬЗЯ)

Правила выбора агентов:
- classifier:classify - для определения категории проблемы
- encoder:embed - для создания эмбеддингов (если нужен поиск)
- researcher:search - для поиска решений в базе знаний
- generator:generate_response - для генерации финального ответа

Для финального ответа ВСЕГДА используй generator:generate_response.

Составь план из 2-3 шагов. Только формат "X. агент:навык → действие", ничего лишнего.`

	systemPrompt = fmt.Sprintf(systemPrompt, agentsText)

	userPrompt := fmt.Sprintf(`Обращение пользователя: "%s"

Составь план обработки.`, text)

	return s.llmClient.Generate(systemPrompt, userPrompt)
}

func (s *OrchestratorService) defaultPlan() string {
	return `1. classifier:classify → category
2. researcher:search → solutions
3. generator:generate_response → answer`
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
