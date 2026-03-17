package main

import (
	"log"
	"orchestrator/internal/config"
	"orchestrator/internal/db"
	"orchestrator/internal/discovery"
	"orchestrator/internal/handlers"
	"orchestrator/internal/services"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
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
		if err := gormDB.AutoMigrate(&db.Dispatcher{}); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}

		// Проверяем, есть ли записи
		var count int64
		gormDB.Model(&db.Dispatcher{}).Count(&count)

		if count == 0 {
			log.Println("No dispatchers found, creating test record...")

			// Создаём тестовую диспетчерскую
			testDispatcher := &db.Dispatcher{
				Name:   "Test Company",
				APIKey: "test-api-key-123",
				Config: datatypes.JSON([]byte(`{
	                "communication_style": "friendly",
	                "confidence_threshold": 0.7,
	                "enable_internet_search": true,
	                "company_context": "Тестовая компания для разработки"
	            }`)),
				Status: "active",
			}

			if err := gormDB.Create(testDispatcher).Error; err != nil {
				log.Printf("Warning: Could not create test dispatcher: %v", err)
			} else {
				log.Printf("Created test(init) dispatcher with ID: %s", testDispatcher.ID)
			}
		}
	}

	// Инициализация репозиториев
	dispatcherRepo := services.NewDispatcherRepository(gormDB)

	// Инициализация discovery клиента
	var discoveryClient discovery.Client
	if cfg.DiscoveryType == "static" {
		dc, err := discovery.NewStaticClient(cfg.StaticAgents)
		if err != nil {
			log.Fatalf("Failed to load static agents: %v", err)
		}
		discoveryClient = dc
	} else {
		log.Fatalf("Unsupported discovery type: %s", cfg.DiscoveryType)
	}

	// Клиент для агентов
	agentClient := services.NewAgentClient(10 * time.Second)

	// 🔥 Инициализация LLM клиента
	llmClient := services.NewLLMClient(cfg.LLMEndpoint, cfg.LLMModel)
	log.Printf("LLM client initialized with model: %s", cfg.LLMModel)

	// Оркестратор сервис с LLM
	orchestratorSvc := services.NewOrchestratorService(
		discoveryClient,
		agentClient,
		dispatcherRepo,
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

	log.Printf("Starting orchestrator on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
