package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Dispatcher struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string         `gorm:"size:255;not null"`
	APIKey    string         `gorm:"size:255;not null"`
	Config    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	Status    string         `gorm:"size:50;default:'active'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
