package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type LLMClient struct {
	httpClient *http.Client
	endpoint   string
	model      string
}

type LLMRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMResponse struct {
	Message Message `json:"message"`
}

func NewLLMClient(endpoint, model string) *LLMClient {
	return &LLMClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   endpoint,
		model:      model,
	}
}

func (c *LLMClient) Generate(systemPrompt, userPrompt string) (string, error) {
	reqBody := LLMRequest{
		Model: c.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Post(c.endpoint, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Ollama format
	if message, ok := result["message"].(map[string]interface{}); ok {
		if content, ok := message["content"].(string); ok {
			return content, nil
		}
	}

	return "", fmt.Errorf("unexpected LLM response format")
}
