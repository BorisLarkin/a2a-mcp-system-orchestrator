package discovery

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"orchestrator/internal/db"
	"sync"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// HybridClient загружает агентов из БД и опрашивает их через /.well-known/agent.json
type HybridClient struct {
	mu         sync.RWMutex
	agents     []Agent
	agentRepo  *gorm.DB // ссылка на БД для обновления статусов
	httpClient *http.Client
}

func NewHybridClient(database *gorm.DB) *HybridClient {
	return &HybridClient{
		agentRepo: database,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Refresh загружает агентов из БД и опрашивает каждого
func (c *HybridClient) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Println("Discovery: loading agents from DB...")

	var dbAgents []db.Agent
	if err := c.agentRepo.Where("status != ?", "deleted").Find(&dbAgents).Error; err != nil {
		log.Printf("Discovery: failed to load agents from DB: %v", err)
		return
	}

	log.Printf("Discovery: loaded %d agents from DB", len(dbAgents))

	// Конвертируем в доменную модель
	c.agents = make([]Agent, 0, len(dbAgents))

	for _, dba := range dbAgents {
		agent := Agent{
			ID:       dba.ID.String(),
			Name:     dba.Name,
			Endpoint: dba.Endpoint,
			Status:   "offline", // по умолчанию, обновим после опроса
		}

		// Устанавливаем DispatcherID
		if dba.DispatcherID != nil {
			agent.DispatcherID = dba.DispatcherID.String()
		}

		// Парсим capabilities
		if dba.Capabilities != nil {
			json.Unmarshal(dba.Capabilities, &agent.Capabilities)
		}

		// Опрашиваем агента
		agentCard, err := c.fetchAgentCard(dba.Endpoint)
		if err != nil {
			log.Printf("Discovery: agent %s (%s) unreachable: %v", dba.Name, dba.Endpoint, err)
			agent.Status = "offline"
			// Обновляем статус в БД
			c.agentRepo.Model(&dba).Updates(map[string]interface{}{
				"status":     "offline",
				"updated_at": time.Now(),
			})
		} else {
			// Обновляем из agent card
			if name, ok := agentCard["name"].(string); ok {
				agent.Name = name
			}
			if caps, ok := agentCard["capabilities"].([]interface{}); ok {
				capStrings := make([]string, len(caps))
				for i, c := range caps {
					capStrings[i] = fmt.Sprint(c)
				}
				agent.Capabilities = capStrings
			}
			if skills, ok := agentCard["skills"].([]interface{}); ok {
				skillList := make([]Skill, 0, len(skills))
				for _, s := range skills {
					if smap, ok := s.(map[string]interface{}); ok {
						skill := Skill{}
						if id, ok := smap["id"].(string); ok {
							skill.ID = id
						}
						if desc, ok := smap["description"].(string); ok {
							skill.Description = desc
						}
						skillList = append(skillList, skill)
					}
				}
				agent.Skills = skillList
			}

			agent.Status = "online"

			// Обновляем в БД
			capJSON, _ := json.Marshal(agent.Capabilities)
			skillsJSON, _ := json.Marshal(agent.Skills)
			metaJSON, _ := json.Marshal(agentCard)

			c.agentRepo.Model(&dba).Updates(map[string]interface{}{
				"name":         agent.Name,
				"capabilities": datatypes.JSON(capJSON),
				"skills":       datatypes.JSON(skillsJSON),
				"status":       "online",
				"metadata":     datatypes.JSON(metaJSON),
				"updated_at":   time.Now(),
			})
		}

		c.agents = append(c.agents, agent)
	}

	onlineCount := 0
	for _, a := range c.agents {
		if a.Status == "online" {
			onlineCount++
		}
		log.Printf("Discovery: agent %s — %s, skills: %v", a.Name, a.Status, getSkillIDs(a.Skills))
	}
	log.Printf("Discovery: refresh complete. %d/%d agents online", onlineCount, len(c.agents))
}

// fetchAgentCard делает GET {endpoint}/.well-known/agent.json
func (c *HybridClient) fetchAgentCard(endpoint string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/.well-known/agent.json", endpoint)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var card map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return card, nil
}

// GetAgents возвращает online-агентов по фильтру capabilities
// GetAgents возвращает online-агентов по фильтру capabilities и dispatcher_id
func (c *HybridClient) GetAgents(dispatcherID string, requiredCapabilities []string) ([]Agent, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fmt.Printf("Getting agents with required capabilities: %v (dispatcher: %s)\n", requiredCapabilities, dispatcherID)

	var result []Agent

	for _, agent := range c.agents {
		if agent.Status != "online" {
			continue
		}

		// Фильтр по диспетчерской:
		// - агенты без dispatcher_id (общие) доступны всем
		// - агенты с dispatcher_id доступны только своей диспетчерской
		if agent.DispatcherID != "" && agent.DispatcherID != dispatcherID {
			continue
		}

		if len(requiredCapabilities) == 0 {
			result = append(result, agent)
			continue
		}

		if hasCapabilities(agent.Capabilities, requiredCapabilities) {
			result = append(result, agent)
		}
	}

	fmt.Printf("Returning %d agents\n", len(result))
	return result, nil
}

func getSkillIDs(skills []Skill) []string {
	ids := make([]string, len(skills))
	for i, s := range skills {
		ids[i] = s.ID
	}
	return ids
}
