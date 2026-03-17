package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AgentClient struct {
	httpClient *http.Client
}

func NewAgentClient(timeout time.Duration) *AgentClient {
	// Увеличиваем таймаут с 10 до 30 секунд для генератора
	return &AgentClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // было 10, стало 30
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
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
// Универсальный метод вызова любого агента
func (c *AgentClient) call(url string, payload interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Calling %s with payload: %s\n", url, string(data))

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
