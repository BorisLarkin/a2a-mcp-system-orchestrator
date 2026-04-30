package services

import (
	"encoding/json"
	"errors"
	"orchestrator/internal/db"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentRepository struct {
	db *gorm.DB
}

func NewAgentRepository(database *gorm.DB) *AgentRepository {
	return &AgentRepository{db: database}
}

// ListOnline возвращает всех online-агентов (опционально фильтр по dispatcher_id)
func (r *AgentRepository) ListOnline(dispatcherID *string) ([]db.Agent, error) {
	var agents []db.Agent
	query := r.db.Where("status = ?", "online")
	if dispatcherID != nil && *dispatcherID != "" {
		query = query.Where("dispatcher_id = ? OR dispatcher_id IS NULL", *dispatcherID)
	}
	err := query.Find(&agents).Error
	return agents, err
}

// ListByDispatcher возвращает агентов конкретной диспетчерской + общих
func (r *AgentRepository) ListByDispatcher(dispatcherID string) ([]db.Agent, error) {
	var agents []db.Agent
	err := r.db.Where(
		"dispatcher_id = ? OR dispatcher_id IS NULL", dispatcherID,
	).Find(&agents).Error
	return agents, err
}

// GetByID возвращает агента по ID
func (r *AgentRepository) GetByID(id string) (*db.Agent, error) {
	var agent db.Agent
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New("invalid agent ID")
	}
	err = r.db.First(&agent, "id = ?", parsedID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("agent not found")
		}
		return nil, err
	}
	return &agent, nil
}

// UpdateStatus обновляет статус и last_heartbeat агента
func (r *AgentRepository) UpdateStatus(id string, status string) error {
	now := time.Now()
	return r.db.Model(&db.Agent{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":         status,
		"last_heartbeat": now,
		"updated_at":     now,
	}).Error
}

// UpdateFromDiscovery обновляет данные агента после опроса /.well-known/agent.json
func (r *AgentRepository) UpdateFromDiscovery(agent *db.Agent) error {
	return r.db.Model(&db.Agent{}).Where("id = ?", agent.ID).Updates(map[string]interface{}{
		"name":         agent.Name,
		"capabilities": agent.Capabilities,
		"skills":       agent.Skills,
		"status":       agent.Status,
		"metadata":     agent.Metadata,
		"updated_at":   time.Now(),
	}).Error
}

// Create создаёт нового агента
func (r *AgentRepository) Create(agent *db.Agent) error {
	return r.db.Create(agent).Error
}

// CreateOrUpdate находит агента по endpoint или создаёт нового
func (r *AgentRepository) CreateOrUpdate(agent *db.Agent) error {
	var existing db.Agent
	err := r.db.Where("endpoint = ?", agent.Endpoint).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(agent).Error
	}
	if err != nil {
		return err
	}
	agent.ID = existing.ID
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"name":         agent.Name,
		"capabilities": agent.Capabilities,
		"skills":       agent.Skills,
		"status":       agent.Status,
		"metadata":     agent.Metadata,
		"updated_at":   time.Now(),
	}).Error
}

// PingAgent обновляет last_heartbeat
func (r *AgentRepository) PingAgent(id string) error {
	now := time.Now()
	return r.db.Model(&db.Agent{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_heartbeat": now,
		"updated_at":     now,
	}).Error
}

// LogRegistration записывает событие регистрации
func (r *AgentRepository) LogRegistration(agentID uuid.UUID, eventType string, sourceIP *string, metadata map[string]interface{}) error {
	metaJSON, _ := json.Marshal(metadata)
	reg := db.AgentRegistration{
		AgentID:   agentID,
		EventType: eventType,
		SourceIP:  sourceIP,
		Metadata:  datatypes.JSON(metaJSON),
	}
	return r.db.Create(&reg).Error
}

// Upsert обновляет или создаёт агента (используется при seed)
func (r *AgentRepository) Upsert(agent *db.Agent) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "endpoint"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "capabilities", "skills", "status", "metadata", "updated_at"}),
	}).Create(agent).Error
}
