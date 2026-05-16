package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"orchestrator/internal/db"
)

type DispatcherHandler struct {
	db *gorm.DB
}

func NewDispatcherHandler(database *gorm.DB) *DispatcherHandler {
	return &DispatcherHandler{db: database}
}

type RegisterDispatcherRequest struct {
	CompanyName string `json:"company_name" binding:"required"`
	Email       string `json:"email" binding:"required"`
}

type RegisterDispatcherResponse struct {
	DispatcherID  string `json:"dispatcher_id"`
	APIKey        string `json:"api_key"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
	Message       string `json:"message"`
}

func (h *DispatcherHandler) Register(c *gin.Context) {
	var req RegisterDispatcherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Генерируем случайный API-ключ
	apiKey := "sk_live_" + randomString(32)
	// Генерируем логин и пароль администратора
	adminUsername := "admin_" + randomString(8)
	adminPassword := randomString(16)
	//hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)

	// Создаём диспетчерскую
	configJSON := datatypes.JSON([]byte(`{
		"communication_style": "friendly",
		"confidence_threshold": 0.7,
		"company_context": ""
	}`))

	dispatcher := db.Dispatcher{
		Name:   req.CompanyName,
		Email:  req.Email,
		APIKey: apiKey,
		Config: configJSON,
		Status: "active",
	}
	if err := h.db.Create(&dispatcher).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create dispatcher: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, RegisterDispatcherResponse{
		DispatcherID:  dispatcher.ID.String(),
		APIKey:        apiKey,
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
		Message:       "Dispatcher registered successfully. Save these credentials — they won't be shown again.",
	})
}

func randomString(length int) string {
	bytes := make([]byte, length/2+2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

func (h *DispatcherHandler) ValidateAPIKey(c *gin.Context) {
	var req struct {
		APIKey       string `json:"api_key" binding:"required"`
		DispatcherID string `json:"dispatcher_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dispatcher db.Dispatcher
	if err := h.db.Where("id = ? AND api_key = ? AND status = ?", req.DispatcherID, req.APIKey, "active").First(&dispatcher).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid": true,
		"dispatcher": gin.H{
			"id":   dispatcher.ID,
			"name": dispatcher.Name,
		},
	})
}

func (h *DispatcherHandler) List(c *gin.Context) {
	var dispatchers []db.Dispatcher
	h.db.Order("created_at DESC").Find(&dispatchers)
	c.JSON(http.StatusOK, gin.H{"dispatchers": dispatchers})
}

func (h *DispatcherHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	var dispatcher db.Dispatcher
	if err := h.db.First(&dispatcher, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Dispatcher not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dispatcher": dispatcher})
}

func (h *DispatcherHandler) UpdateConfig(c *gin.Context) {
	id := c.Param("id")
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	configJSON, _ := json.Marshal(req)
	h.db.Model(&db.Dispatcher{}).Where("id = ?", id).Update("config", datatypes.JSON(configJSON))

	c.JSON(200, gin.H{"message": "Config updated"})
}
