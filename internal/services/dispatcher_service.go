package services

import (
	"errors"
	"orchestrator/internal/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DispatcherRepository struct {
	db *gorm.DB
}

func NewDispatcherRepository(db *gorm.DB) *DispatcherRepository {
	return &DispatcherRepository{db: db}
}

// GetByID возвращает диспетчерскую по её UUID
func (r *DispatcherRepository) GetByID(id string) (*db.Dispatcher, error) {
	var dispatcher db.Dispatcher
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New("invalid dispatcher ID format")
	}
	err = r.db.Where("id = ?", parsedID).First(&dispatcher).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("dispatcher not found")
		}
		return nil, err
	}
	return &dispatcher, nil
}

// GetByAPIKey возвращает диспетчерскую по API ключу (для аутентификации)
func (r *DispatcherRepository) GetByAPIKey(apiKey string) (*db.Dispatcher, error) {
	var dispatcher db.Dispatcher
	err := r.db.Where("api_key = ?", apiKey).First(&dispatcher).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid API key")
		}
		return nil, err
	}
	return &dispatcher, nil
}

// Create добавляет новую диспетчерскую (может пригодиться для админки)
func (r *DispatcherRepository) Create(dispatcher *db.Dispatcher) error {
	return r.db.Create(dispatcher).Error
}

// Update обновляет конфигурацию и другие поля
func (r *DispatcherRepository) Update(dispatcher *db.Dispatcher) error {
	return r.db.Save(dispatcher).Error
}
