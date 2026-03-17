package handlers

import (
	"net/http"
	"orchestrator/internal/services"

	"github.com/gin-gonic/gin"
)

type TicketHandler struct {
	orchestrator *services.OrchestratorService
}

func NewTicketHandler(os *services.OrchestratorService) *TicketHandler {
	return &TicketHandler{orchestrator: os}
}

type processTicketRequest struct {
	Text         string                 `json:"text" binding:"required"`
	DispatcherID string                 `json:"dispatcher_id" binding:"required"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

func (h *TicketHandler) Process(c *gin.Context) {
	var req processTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.orchestrator.Process(c.Request.Context(), &services.ProcessTicketRequest{
		Text:         req.Text,
		DispatcherID: req.DispatcherID,
		Metadata:     req.Metadata,
	})
	if err != nil {
		// логируем
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}
