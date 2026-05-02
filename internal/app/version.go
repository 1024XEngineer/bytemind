package app

import (
	"os/exec"
	"strings"
)

// buildVersion is injected at release build time.
// Default stays "dev" for local development and go run workflows.
var buildVersion = "dev"

func CurrentVersion() string {
	version := strings.TrimSpace(buildVersion)
	if version != "" && version != "dev" {
		return version
	}
	if gitVersion := strings.TrimSpace(gitDescribe()); gitVersion != "" {
		return gitVersion
	}
	if version != "" {
		return version
	}
	return "dev"
}

var gitDescribe = func() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return ""
	}
	return string(out)
}
