package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"orchestrator/internal/db"
	"orchestrator/internal/models"
	"time"

	"gorm.io/datatypes"
)

type A2AClient struct {
	httpClient *http.Client
	ticketRepo *TicketRepository
}

func NewA2AClient(timeout time.Duration, ticketRepo *TicketRepository) *A2AClient {
	return &A2AClient{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		ticketRepo: ticketRepo,
	}
}

// SendTask отправляет задачу агенту и ожидает синхронный ответ
func (c *A2AClient) SendTask(agentEndpoint string, skillID string, input map[string]interface{}) (*models.A2ATaskResponse, error) {
	req := models.A2ATaskRequest{
		SkillID: skillID,
		Input:   input,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/tasks/send", agentEndpoint)

	fmt.Printf("A2A → Calling %s with skill '%s', payload: %s\n", url, skillID, string(body))

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("post to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return nil, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, errBody.String())
	}

	var taskResp models.A2ATaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return nil, fmt.Errorf("decode response from %s: %w", url, err)
	}

	fmt.Printf("A2A ← Response from %s: task_id=%s, status=%s, output_keys=%v\n",
		url, taskResp.TaskID, taskResp.Status, getMapKeys(taskResp.Output))

	if taskResp.Status == "failed" {
		return &taskResp, fmt.Errorf("task failed: %s", taskResp.Error)
	}

	// Логируем A2A вызов
	if c.ticketRepo != nil {
		reqJSON, _ := json.Marshal(req)
		respJSON, _ := json.Marshal(taskResp)
		status := taskResp.Status
		var taskID *string
		if taskResp.TaskID != "" {
			taskID = &taskResp.TaskID
		}
		call := &db.A2ACall{
			SkillID:  &skillID,
			Request:  datatypes.JSON(reqJSON),
			Response: datatypes.JSON(respJSON),
			TaskID:   taskID,
			Status:   &status,
		}
		if taskResp.Status == "failed" {
			call.Error = &taskResp.Error
		}
		c.ticketRepo.SaveA2ACall(call)
	}

	return &taskResp, nil
}

// getMapKeys возвращает список ключей мапы для логгирования
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
