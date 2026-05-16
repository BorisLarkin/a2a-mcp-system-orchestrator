package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type KnowledgeHandler struct {
	encoderURL string
	qdrantURL  string
}

func NewKnowledgeHandler(encoderURL, qdrantURL string) *KnowledgeHandler {
	return &KnowledgeHandler{encoderURL: encoderURL, qdrantURL: qdrantURL}
}

type AddDocumentRequest struct {
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content" binding:"required"`
	Category string `json:"category"`
	TicketID string `json:"ticket_id,omitempty"`
}

func (h *KnowledgeHandler) AddDocument(c *gin.Context) {
	var req AddDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 1. Получить эмбеддинг через MCP encoder
	vector, err := getEmbedding(h.encoderURL, req.Content)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get embedding: " + err.Error()})
		return
	}

	// 2. Сохранить в Qdrant через MCP
	err = upsertDocument(h.qdrantURL, req.Title, req.Content, req.Category, vector)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save document: " + err.Error()})
		return
	}

	c.JSON(201, gin.H{"message": "Document added to knowledge base"})
}

func getEmbedding(encoderURL, text string) ([]float64, error) {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0", "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "embed",
			"arguments": map[string]interface{}{"texts": []string{text}},
		}, "id": 1,
	}
	body, _ := json.Marshal(rpcReq)
	resp, err := http.Post(encoderURL+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	embeddings := result["result"].(map[string]interface{})["embeddings"].([]interface{})
	vec := make([]float64, len(embeddings[0].([]interface{})))
	for i, v := range embeddings[0].([]interface{}) {
		vec[i] = v.(float64)
	}
	return vec, nil
}

func upsertDocument(qdrantURL, title, content, category string, vector []float64) error {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0", "method": "tools/call",
		"params": map[string]interface{}{
			"name": "upsert",
			"arguments": map[string]interface{}{
				"documents": []map[string]interface{}{{
					"id": uuid.NewString(), "title": title, "content": content,
					"category": category, "source": "manual", "vector": vector,
				}},
			},
		}, "id": 1,
	}
	body, _ := json.Marshal(rpcReq)
	resp, err := http.Post(qdrantURL+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
