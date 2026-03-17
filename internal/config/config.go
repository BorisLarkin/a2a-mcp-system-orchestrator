package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv        string `env:"APP_ENV" envDefault:"development"`
	DBHost        string `env:"DB_HOST" envDefault:"localhost"`
	DBPort        int    `env:"DB_PORT" envDefault:"5432"`
	DBUser        string `env:"DB_USER" envDefault:"orchestrator"`
	DBPassword    string `env:"DB_PASSWORD" envDefault:"orchestrator_pass"`
	DBName        string `env:"DB_NAME" envDefault:"orchestrator"`
	RedisHost     string `env:"REDIS_HOST" envDefault:"localhost"`
	RedisPort     int    `env:"REDIS_PORT" envDefault:"6379"`
	DiscoveryType string `env:"DISCOVERY_TYPE" envDefault:"static"`
	StaticAgents  string `env:"STATIC_AGENTS_FILE" envDefault:"app/config/agents.json"`
	LogLevel      string `env:"LOG_LEVEL" envDefault:"info"`
	LLMEndpoint   string `env:"LLM_ENDPOINT" envDefault:"http://ollama:11434/api/chat"`
	LLMModel      string `env:"LLM_MODEL" envDefault:"phi3:mini"`
}

func Load() (*Config, error) {
	_ = godotenv.Load() // загружаем .env если есть
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
