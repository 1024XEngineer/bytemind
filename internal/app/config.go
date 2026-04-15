package app

import (
	"fmt"
	"strconv"
	"strings"

	"bytemind/internal/config"
)

// ConfigRequest defines workspace config load plus CLI runtime overrides.
type ConfigRequest struct {
	Workspace             string
	ConfigPath            string
	ModelOverride         string
	StreamOverride        string
	MaxIterationsOverride int
}

func LoadRuntimeConfig(req ConfigRequest) (config.Config, error) {
	cfg, err := config.Load(strings.TrimSpace(req.Workspace), strings.TrimSpace(req.ConfigPath))
	if err != nil {
		return cfg, err
	}
	if req.ModelOverride != "" {
		cfg.Provider.Model = req.ModelOverride
	}
	if req.StreamOverride != "" {
		parsed, err := strconv.ParseBool(req.StreamOverride)
		if err != nil {
			return cfg, fmt.Errorf("invalid -stream value: %w", err)
		}
		cfg.Stream = parsed
	}
	if req.MaxIterationsOverride > 0 {
		cfg.MaxIterations = req.MaxIterationsOverride
	}
	return cfg, nil
}
