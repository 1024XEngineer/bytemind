package app

import "context"

type ErrorCode string

const (
	ErrCodeInvalidConfig   ErrorCode = "invalid_config"
	ErrCodeBuildFailed     ErrorCode = "build_failed"
	ErrCodeStartFailed     ErrorCode = "start_failed"
	ErrCodeStopFailed      ErrorCode = "stop_failed"
	ErrCodeShutdownTimeout ErrorCode = "shutdown_timeout"
)

type ConfigSource struct {
	FilePath   string
	EnvPrefix  string
	Args       []string
	WorkingDir string
}

type Config struct {
	WorkspaceRoot string
	SessionMode   string
	LogLevel      string
}

type ConfigLoader interface {
	Load(ctx context.Context, source ConfigSource) (Config, error)
}

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Ready() <-chan struct{}
}

type ModuleSet struct {
	Storage    Component
	Session    Component
	Policy     Component
	Tools      Component
	Extensions Component
	Context    Component
	Provider   Component
	Runtime    Component
	Agent      Component
}

type Bootstrapper interface {
	Build(ctx context.Context, cfg Config) (ModuleSet, error)
}

type LifecycleManager interface {
	Start(ctx context.Context, modules ModuleSet) error
	Stop(ctx context.Context, modules ModuleSet) error
}

type Application interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

