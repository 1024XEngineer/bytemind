package extensions

import "context"

// Loader loads extension providers.
type Loader interface {
	LoadAll(ctx context.Context) ([]Provider, error)
}

// Provider declares tools from one extension source.
type Provider interface {
	Name() string
	Tools(ctx context.Context) ([]ToolAdapter, error)
}

// ToolAdapter maps extension tool into tool contract.
type ToolAdapter interface {
	ToolName() string
}
