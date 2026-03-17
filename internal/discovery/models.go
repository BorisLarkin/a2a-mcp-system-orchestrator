package discovery

type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Endpoint     string   `json:"endpoint"`      // base URL для вызовов
	Capabilities []string `json:"capabilities"`  // например, ["classification", "embedding"]
	Status       string   `json:"status"`        // online/offline
	Metadata     map[string]interface{} `json:"metadata"`
}