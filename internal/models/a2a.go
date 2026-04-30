package models

// A2ATaskRequest — стандартизированный запрос к агенту
type A2ATaskRequest struct {
	SkillID string                 `json:"skill_id"`
	Input   map[string]interface{} `json:"input"`
}

// A2ATaskResponse — стандартизированный ответ агента
type A2ATaskResponse struct {
	TaskID string                 `json:"task_id,omitempty"`
	Status string                 `json:"status"` // "completed", "failed", "in_progress"
	Output map[string]interface{} `json:"output,omitempty"`
	Error  string                 `json:"error,omitempty"`
}
