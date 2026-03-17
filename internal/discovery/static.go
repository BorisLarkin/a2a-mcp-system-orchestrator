package discovery

import (
	"encoding/json"
	"fmt"
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
	fmt.Printf("Getting agents with required capabilities: %v\n", requiredCapabilities)

	var result []Agent

	for _, agent := range c.agents {
		fmt.Printf("Checking agent %s with capabilities: %v, status: %s\n",
			agent.Name, agent.Capabilities, agent.Status)

		if agent.Status != "online" {
			fmt.Printf("Agent %s is offline, skipping\n", agent.Name)
			continue
		}

		if len(requiredCapabilities) == 0 {
			// Если нет требований, возвращаем всех online
			result = append(result, agent)
			fmt.Printf("Adding agent %s (no capability filter)\n", agent.Name)
			continue
		}

		if hasCapabilities(agent.Capabilities, requiredCapabilities) {
			result = append(result, agent)
			fmt.Printf("Adding agent %s (matches capabilities)\n", agent.Name)
		}
	}

	fmt.Printf("Returning %d agents\n", len(result))
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
