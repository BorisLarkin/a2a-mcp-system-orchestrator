package discovery

type Client interface {
	// GetAgents возвращает список агентов, доступных для данной диспетчерской
	GetAgents(dispatcherID string, requiredCapabilities []string) ([]Agent, error)
	// RegisterAgent (если нужно)
	// HealthCheck...
}