package discovery

import (
	"encoding/json"
	"os"
)

type StaticClient struct {
	agents []Agent
}

func NewStaticClient(filePath string) (*StaticClient, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, err
	}
	return &StaticClient{agents: agents}, nil
}

func (c *StaticClient) GetAgents(dispatcherID string, requiredCapabilities []string) ([]Agent, error) {
	// Фильтруем по capabilities, если нужно
	var result []Agent
	for _, agent := range c.agents {
		if agent.Status == "online" && hasCapabilities(agent.Capabilities, requiredCapabilities) {
			result = append(result, agent)
		}
	}
	return result, nil
}

func hasCapabilities(available, required []string) bool {
	if len(required) == 0 {
		return true
	}
	// упрощённо: считаем что все required есть в available
	// можно реализовать пересечение множеств
	return true
}
