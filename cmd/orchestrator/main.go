package main

import (
	"log"
	"net/http"
	"orchestrator/internal/config"
	"orchestrator/internal/db"
	"orchestrator/internal/discovery"
	"orchestrator/internal/handlers"
	"orchestrator/internal/services"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Подключение к PostgreSQL
	gormDB, err := db.NewPostgresConnection(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	// Автомиграция (в dev)
	if cfg.AppEnv == "development" {
		log.Println("Running auto-migration...")

		// Сначала создаём таблицу (если нет)
		if err := gormDB.AutoMigrate(
			&db.Dispatcher{},
			&db.Agent{},
			&db.AgentRegistration{},
			&db.Ticket{},
			&db.A2ACall{},
			&db.KnowledgeBase{},
			&db.DispatcherConfig{},
		); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}

		// Проверяем, есть ли записи
		var count int64
		gormDB.Model(&db.Dispatcher{}).Count(&count)

		if count == 0 {
			log.Println("No dispatchers found, skipping...")
		}
		// Проверяем и создаём seed-агентов
		var agentCount int64
		gormDB.Model(&db.Agent{}).Count(&agentCount)
		if agentCount == 0 {
			log.Println("No agents found, creating seed agents...")
			seedAgents := []db.Agent{
				{
					Name:         "rubert-classifier",
					Endpoint:     "http://100.87.189.74:9001",
					AgentType:    "classifier",
					Capabilities: datatypes.JSON([]byte(`["classification"]`)),
					Skills:       datatypes.JSON([]byte(`[{"id":"classify","description":"Classify user request into problem category"},{"id":"extract_entities","description":"Extract named entities from text"}]`)),
					Status:       "online",
					Metadata:     datatypes.JSON([]byte(`{"model":"rubert-tiny2"}`)),
				},
				{
					Name:         "sentence-encoder",
					Endpoint:     "http://100.87.189.74:9102",
					AgentType:    "encoder",
					Capabilities: datatypes.JSON([]byte(`["embedding"]`)),
					Skills:       datatypes.JSON([]byte(`[{"id":"embed","description":"Create vector embeddings from texts"}]`)),
					Status:       "online",
					Metadata:     datatypes.JSON([]byte(`{"model":"paraphrase-multilingual-MiniLM-L12-v2"}`)),
				},
				{
					Name:         "llm-generator",
					Endpoint:     "http://100.87.189.74:9003",
					AgentType:    "generator",
					Capabilities: datatypes.JSON([]byte(`["generation"]`)),
					Skills:       datatypes.JSON([]byte(`[{"id":"generate_response","description":"Generate final answer"},{"id":"format_response","description":"Format and adjust tone"}]`)),
					Status:       "online",
					Metadata:     datatypes.JSON([]byte(`{"model":"llama3.2"}`)),
				},
				{
					Name:         "researcher",
					Endpoint:     "http://100.87.189.74:9002",
					AgentType:    "researcher",
					Capabilities: datatypes.JSON([]byte(`["search"]`)),
					Skills:       datatypes.JSON([]byte(`[{"id":"search","description":"Search knowledge base and internet"},{"id":"search_knowledge_base","description":"Search only local knowledge base"}]`)),
					Status:       "online",
					Metadata:     datatypes.JSON([]byte(`{"type":"mock"}`)),
				},
			}
			for _, a := range seedAgents {
				if err := gormDB.Create(&a).Error; err != nil {
					log.Printf("Warning: Could not create seed agent %s: %v", a.Name, err)
				}
			}
			log.Printf("Created %d seed agents", len(seedAgents))
		}
	}

	// Инициализация репозиториев
	dispatcherRepo := services.NewDispatcherRepository(gormDB)
	agentRepo := services.NewAgentRepository(gormDB)
	ticketRepo := services.NewTicketRepository(gormDB)

	// Инициализация discovery клиента
	// Инициализация hybrid discovery (из БД + опрос /.well-known/agent.json)
	var discoveryClient discovery.Client
	if cfg.DiscoveryType == "hybrid" || cfg.DiscoveryType == "dynamic" {
		dc := discovery.NewHybridClient(gormDB)
		dc.Refresh() // первый опрос при старте
		discoveryClient = dc
		log.Println("Discovery: hybrid mode active — agents from DB + /.well-known/agent.json")

		// Периодическое обновление
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.DiscoveryRefreshInterval) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				log.Println("Discovery: periodic refresh triggered")
				dc.Refresh()
			}
		}()
	} else {
		// Fallback: статический из файла
		dc, err := discovery.NewStaticClient(cfg.StaticAgents)
		if err != nil {
			log.Fatalf("Failed to load static agents: %v", err)
		}
		discoveryClient = dc
	}

	// Клиент для агентов
	agentClient := services.NewAgentClient(30 * time.Second)
	// A2A-клиент для унифицированных вызовов агентов
	a2aClient := services.NewA2AClient(30*time.Second, ticketRepo)

	// 🔥 Инициализация LLM клиента
	llmClient := services.NewLLMClient(cfg.LLMEndpoint, cfg.LLMModel)
	log.Printf("LLM client initialized with model: %s", cfg.LLMModel)

	// Оркестратор сервис с LLM
	orchestratorSvc := services.NewOrchestratorService(
		discoveryClient,
		agentClient,
		a2aClient,
		dispatcherRepo,
		agentRepo,
		ticketRepo,
		llmClient,
	)

	// HTTP handler
	ticketHandler := handlers.NewTicketHandler(orchestratorSvc)

	// Gin роутер
	r := gin.Default()
	r.POST("/process-ticket", ticketHandler.Process)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API для управления агентами (защищён API-ключом диспетчерской)
	agentGroup := r.Group("/api/v1/agents")
	agentGroup.Use(apiKeyAuthMiddleware(gormDB))
	{
		agentHandler := handlers.NewAgentHandler(gormDB, discoveryClient)
		agentGroup.POST("", agentHandler.RegisterAgent)
		agentGroup.GET("", agentHandler.ListAgents)
		agentGroup.DELETE("/:id", agentHandler.DeleteAgent)
	}

	// SaaS Admin middleware (для регистрации диспетчерских)
	saasAdminKey := os.Getenv("SAAS_ADMIN_KEY")
	if saasAdminKey == "" {
		saasAdminKey = "super_secret_admin_key" // fallback для dev
	}
	dispatcherHandler := handlers.NewDispatcherHandler(gormDB)
	saasAdminGroup := r.Group("/api/v1/admin/dispatchers")
	saasAdminGroup.Use(func(c *gin.Context) {
		key := c.GetHeader("X-Admin-Key")
		if key != saasAdminKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid admin key"})
			c.Abort()
			return
		}
		c.Next()
	})
	{
		saasAdminGroup.POST("", dispatcherHandler.Register)
		saasAdminGroup.GET("", dispatcherHandler.List)
	}

	// Публичный эндпоинт для валидации ключа (используется клиентом при подключении)
	r.POST("/api/v1/dispatchers/validate", dispatcherHandler.ValidateAPIKey)

	log.Printf("Starting orchestrator on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func apiKeyAuthMiddleware(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(401, gin.H{"error": "X-API-Key required"})
			c.Abort()
			return
		}
		// Ищем диспетчерскую по API-ключу
		var dispatcher db.Dispatcher
		if err := database.Where("api_key = ?", apiKey).First(&dispatcher).Error; err != nil {
			c.JSON(401, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}
		c.Set("dispatcher_id", dispatcher.ID)
		c.Next()
	}
}
