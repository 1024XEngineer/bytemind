package mention

type AgentSource interface {
	ListAgents() []AgentEntry
}

type AgentEntry struct {
	Name        string
	Description string
	Scope       string
}
