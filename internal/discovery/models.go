package discovery

import "encoding/json"

type Skill struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema,omitempty"`
	OutputSchema map[string]interface{} `json:"output_schema,omitempty"`
}

type Agent struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Endpoint     string                 `json:"endpoint"`
	Capabilities []string               `json:"capabilities"`
	Status       string                 `json:"status"`
	Skills       []Skill                `json:"skills,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// GetSkillByID возвращает навык по ID
func (a *Agent) GetSkillByID(skillID string) *Skill {
	for _, s := range a.Skills {
		if s.ID == skillID {
			return &s
		}
	}
	return nil
}

// RequiredInputFields возвращает список обязательных полей из input_schema навыка
func (s *Skill) RequiredInputFields() []string {
	if s.InputSchema == nil {
		return nil
	}
	props, _ := s.InputSchema["properties"].(map[string]interface{})
	required, _ := s.InputSchema["required"].([]interface{})

	fields := make([]string, 0)
	if required != nil {
		for _, r := range required {
			if fieldName, ok := r.(string); ok {
				fields = append(fields, fieldName)
			}
		}
	}
	// Если required не указан — возвращаем все properties
	if len(fields) == 0 && props != nil {
		for fieldName := range props {
			fields = append(fields, fieldName)
		}
	}
	return fields
}

// AllInputFields возвращает все поля из input_schema (не только required)
func (s *Skill) AllInputFields() []string {
	if s.InputSchema == nil {
		return nil
	}
	props, _ := s.InputSchema["properties"].(map[string]interface{})
	fields := make([]string, 0, len(props))
	for fieldName := range props {
		fields = append(fields, fieldName)
	}
	return fields
}

// ConfidenceField возвращает имя поля confidence из output_schema, если указано
func (s *Skill) ConfidenceField() string {
	if s.OutputSchema == nil {
		return ""
	}
	// Явное указание confidence_field
	if cf, ok := s.OutputSchema["confidence_field"].(string); ok {
		return cf
	}
	// Ищем "confidence" в properties
	props, _ := s.OutputSchema["properties"].(map[string]interface{})
	if _, hasConf := props["confidence"]; hasConf {
		return "confidence"
	}
	return ""
}

// ToJSON сериализует Agent в JSON (для логов/отладки)
func (a *Agent) ToJSON() string {
	data, _ := json.Marshal(a)
	return string(data)
}
