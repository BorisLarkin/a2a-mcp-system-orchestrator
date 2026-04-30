package discovery

type Skill struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema,omitempty"`
	OutputSchema map[string]interface{} `json:"output_schema,omitempty"`
}

type Agent struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Endpoint     string                 `json:"endpoint"`     // base URL для вызовов
	Capabilities []string               `json:"capabilities"` // например, ["classification", "embedding"]
	Status       string                 `json:"status"`       // online/offline
	Skills       []Skill                `json:"skills,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
}
