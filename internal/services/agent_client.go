package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AgentClient struct {
	httpClient *http.Client
}

func NewAgentClient(timeout time.Duration) *AgentClient {
	return &AgentClient{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// CallClassification вызывает агент-классификатор
func (c *AgentClient) CallClassification(agentURL, text string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"text": text,
	}

	fmt.Printf("🔍 Calling classifier at %s with text: %.50s\n", agentURL, text)

	result, err := c.call(agentURL+"/classify", payload)
	if err != nil {
		return nil, err
	}

	// Логируем полный ответ классификатора
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("📥 Classifier response: %s\n", string(resultJSON))

	return result, nil
}

// CallEmbedding вызывает агент энкодера
func (c *AgentClient) CallEmbedding(agentURL, text string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"texts": []string{text},
	}
	return c.call(agentURL+"/embed", payload)
}

// универсальный метод вызова
func (c *AgentClient) call(url string, payload interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
