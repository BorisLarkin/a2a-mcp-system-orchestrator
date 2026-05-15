package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"orchestrator/internal/db"
	"orchestrator/internal/discovery"
)

type AgentHandler struct {
	db              *gorm.DB
	discoveryClient discovery.Client
}

func NewAgentHandler(database *gorm.DB, dc discovery.Client) *AgentHandler {
	return &AgentHandler{db: database, discoveryClient: dc}
}

// RegisterAgent — регистрация нового агента (с проверкой Agent Card)
func (h *AgentHandler) RegisterAgent(c *gin.Context) {
	dispatcherID, _ := c.Get("dispatcher_id")

	var req struct {
		Endpoint     string `json:"endpoint" binding:"required"`
		DispatcherID string `json:"dispatcher_id"` // опционально, если от local-proxy
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Определяем ID диспетчерской
	var dispID uuid.UUID
	if req.DispatcherID != "" {
		parsed, err := uuid.Parse(req.DispatcherID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid dispatcher_id"})
			return
		}
		dispID = parsed
	} else {
		dispID = dispatcherID.(uuid.UUID)
	}

	// Проверяем, доступна ли карточка агента — запрос идёт от оркестратора
	card, err := fetchAgentCard(req.Endpoint)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Agent unreachable",
			"details": err.Error(),
			"hint":    "Make sure the agent is running and accessible from the orchestrator's network",
		})
		return
	}

	// Парсим capabilities и skills
	caps := getStringArray(card, "capabilities")
	skills := getSkillsArray(card, "skills")

	agent := db.Agent{
		DispatcherID:  &dispID,
		Name:          getString(card, "name"),
		Endpoint:      req.Endpoint,
		AgentType:     getString(card, "type"),
		Capabilities:  toJSON(caps),
		Skills:        toJSON(skills),
		Status:        "online",
		Metadata:      toJSON(card),
		LastHeartbeat: timePtr(time.Now()),
	}

	if err := h.db.Create(&agent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save agent"})
		return
	}

	// Обновляем Discovery немедленно
	if hc, ok := h.discoveryClient.(*discovery.HybridClient); ok {
		hc.Refresh()
	}

	c.JSON(http.StatusCreated, gin.H{
		"agent":   agent,
		"message": "Agent registered successfully",
	})
}

// ListAgents — список агентов диспетчерской + общих
func (h *AgentHandler) ListAgents(c *gin.Context) {
	dispatcherID, _ := c.Get("dispatcher_id")

	var agents []db.Agent
	h.db.Where("dispatcher_id = ? OR dispatcher_id IS NULL", dispatcherID).
		Order("agent_type, name").Find(&agents)

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// DeleteAgent — удаление агента
func (h *AgentHandler) DeleteAgent(c *gin.Context) {
	dispatcherID, _ := c.Get("dispatcher_id")
	agentID := c.Param("id")

	result := h.db.Where("id = ? AND dispatcher_id = ?", agentID, dispatcherID).Delete(&db.Agent{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found or not owned by dispatcher"})
		return
	}

	if hc, ok := h.discoveryClient.(*discovery.HybridClient); ok {
		hc.Refresh()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted"})
}

// --- Вспомогательные функции ---

func fetchAgentCard(endpoint string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint + "/.well-known/agent.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var card map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&card)
	return card, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toJSON(v interface{}) datatypes.JSON {
	data, _ := json.Marshal(v)
	return datatypes.JSON(data)
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	return &id
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func getStringArray(m map[string]interface{}, key string) []string {
	arr, ok := m[key].([]interface{})
	if !ok {
		return []string{}
	}
	result := make([]string, len(arr))
	for i, v := range arr {
		result[i] = fmt.Sprint(v)
	}
	return result
}

func getSkillsArray(m map[string]interface{}, key string) []map[string]interface{} {
	arr, ok := m[key].([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}
	result := make([]map[string]interface{}, len(arr))
	for i, v := range arr {
		if smap, ok := v.(map[string]interface{}); ok {
			result[i] = smap
		}
	}
	return result
}
