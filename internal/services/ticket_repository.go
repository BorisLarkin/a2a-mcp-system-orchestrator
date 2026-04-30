package services

import (
	"encoding/json"
	"orchestrator/internal/db"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type TicketRepository struct {
	db *gorm.DB
}

func NewTicketRepository(database *gorm.DB) *TicketRepository {
	return &TicketRepository{db: database}
}

// Create сохраняет тикет в БД
func (r *TicketRepository) Create(ticket *db.Ticket) error {
	return r.db.Create(ticket).Error
}

// UpdateStatus обновляет статус тикета
func (r *TicketRepository) UpdateStatus(id string, status string) error {
	return r.db.Model(&db.Ticket{}).Where("id = ?", id).Update("status", status).Error
}

// SaveA2ACall сохраняет запись о A2A вызове
func (r *TicketRepository) SaveA2ACall(call *db.A2ACall) error {
	return r.db.Create(call).Error
}

// Helper для конвертации map в JSONB
func toJSONB(v interface{}) datatypes.JSON {
	data, _ := json.Marshal(v)
	return datatypes.JSON(data)
}
