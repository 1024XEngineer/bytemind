package app

import "strings"

// buildVersion is injected at release build time.
// Default stays "dev" for local development and go run workflows.
var buildVersion = "dev"

func CurrentVersion() string {
	return strings.TrimSpace(buildVersion)
}
