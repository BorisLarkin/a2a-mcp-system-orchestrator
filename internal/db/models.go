package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ==================== Dispatcher ====================

type Dispatcher struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string         `gorm:"size:255;not null"`
	APIKey    string         `gorm:"size:255;not null" json:"-"`
	Config    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	Status    string         `gorm:"size:50;default:'active'"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Dispatcher) TableName() string { return "dispatchers" }

// ==================== Agent ====================

type Agent struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DispatcherID  *uuid.UUID     `gorm:"type:uuid"`
	Name          string         `gorm:"size:255;not null"`
	Endpoint      string         `gorm:"size:512;not null"`
	AgentType     string         `gorm:"size:100;not null"`
	Capabilities  datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb"`
	Skills        datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb"`
	Status        string         `gorm:"size:50;default:'pending'"`
	AuthToken     *string        `gorm:"size:512"`
	LastHeartbeat *time.Time
	Metadata      datatypes.JSON `gorm:"type:jsonb;default:'{}'::jsonb"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (Agent) TableName() string { return "agents" }

// ==================== AgentRegistration ====================

type AgentRegistration struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AgentID   uuid.UUID      `gorm:"type:uuid;not null"`
	EventType string         `gorm:"size:50;not null"`
	SourceIP  *string        `gorm:"size:45"`
	Metadata  datatypes.JSON `gorm:"type:jsonb;default:'{}'::jsonb"`
	CreatedAt time.Time
}

func (AgentRegistration) TableName() string { return "agent_registrations" }

// ==================== Ticket ====================

type Ticket struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DispatcherID     *uuid.UUID     `gorm:"type:uuid"`
	ExternalID       *string        `gorm:"size:255"`
	Text             string         `gorm:"type:text;not null"`
	Channel          string         `gorm:"size:50;default:'api'"`
	Classification   datatypes.JSON `gorm:"type:jsonb"`
	Embedding        datatypes.JSON `gorm:"type:jsonb"`
	FinalResponse    *string        `gorm:"type:text"`
	Plan             *string        `gorm:"type:text"`
	ExecutionLog     datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb"`
	AgentsUsed       datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb"`
	Status           string         `gorm:"size:50;default:'received'"`
	Confidence       *float64
	SuggestedTeam    *string        `gorm:"size:100"`
	Metadata         datatypes.JSON `gorm:"type:jsonb;default:'{}'::jsonb"`
	ProcessingTimeMs *int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Ticket) TableName() string { return "tickets" }

// ==================== A2ACall ====================

type A2ACall struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TicketID   *uuid.UUID     `gorm:"type:uuid"`
	AgentID    *uuid.UUID     `gorm:"type:uuid"`
	SkillID    *string        `gorm:"size:100"`
	Request    datatypes.JSON `gorm:"type:jsonb"`
	Response   datatypes.JSON `gorm:"type:jsonb"`
	TaskID     *string        `gorm:"size:255"`
	Status     *string        `gorm:"size:50"`
	Error      *string        `gorm:"type:text"`
	DurationMs *int
	RetryCount int `gorm:"default:0"`
	CreatedAt  time.Time
}

func (A2ACall) TableName() string { return "a2a_calls" }

// ==================== KnowledgeBase ====================

type KnowledgeBase struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DispatcherID  *uuid.UUID     `gorm:"type:uuid"`
	Title         *string        `gorm:"size:500"`
	Content       string         `gorm:"type:text;not null"`
	Category      *string        `gorm:"size:100"`
	Tags          datatypes.JSON `gorm:"type:jsonb;default:'[]'::jsonb"`
	Source        *string        `gorm:"size:50"`
	QdrantPointID *uuid.UUID     `gorm:"type:uuid"`
	Embedded      bool           `gorm:"default:false"`
	UsageCount    int            `gorm:"default:0"`
	SuccessRate   *float64       `gorm:"type:decimal(3,2)"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (KnowledgeBase) TableName() string { return "knowledge_base" }

// ==================== DispatcherConfig ====================

type DispatcherConfig struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DispatcherID uuid.UUID      `gorm:"type:uuid;not null"`
	Config       datatypes.JSON `gorm:"type:jsonb;not null"`
	Version      int            `gorm:"not null;default:1"`
	Comment      *string        `gorm:"type:text"`
	ChangedBy    *string        `gorm:"size:255"`
	CreatedAt    time.Time
}

func (DispatcherConfig) TableName() string { return "dispatcher_configs" }
