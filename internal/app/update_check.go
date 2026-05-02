package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
)

const (
	updateCheckInterval   = 24 * time.Hour
	updateCheckRepository = "1024XEngineer/bytemind"
)

type updateCheckState struct {
	LastCheckedAt time.Time `json:"last_checked_at"`
	LatestVersion string    `json:"latest_version"`
}

type updateCheckResult struct {
	LatestVersion string
	Checked       bool
}

var (
	updateCheckNow                = func() time.Time { return time.Now().UTC() }
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		return fetchLatestVersionFromGitHub(currentVersion)
	}
	updateCheckLatestReleaseURL = "https://api.github.com/repos/" + updateCheckRepository + "/releases/latest"
	updateCheckHTTPClient       = &http.Client{Timeout: 2 * time.Second}
)

func maybePrintUpdateReminder(cfg config.Config, w io.Writer) {
	if w == nil || !cfg.UpdateCheck.Enabled {
		return
	}

	currentVersion := normalizeVersionTag(CurrentVersion())
	if currentVersion == "" {
		return
	}

	result, err := latestVersionWithCache(updateCheckNow(), currentVersion)
	if err != nil || !result.Checked {
		return
	}
	if !isVersionNewer(result.LatestVersion, currentVersion) {
		return
	}

	fmt.Fprintf(w, "update available: %s -> %s. Re-run the install command to upgrade.\n", currentVersion, result.LatestVersion)
}

func latestVersionWithCache(now time.Time, currentVersion string) (updateCheckResult, error) {
	statePath, err := resolveUpdateCheckStatePath()
	if err != nil {
		return updateCheckResult{}, err
	}

	state, hasState := readUpdateCheckState(statePath)
	if hasState && !state.LastCheckedAt.IsZero() && now.Sub(state.LastCheckedAt) < updateCheckInterval {
		return updateCheckResult{LatestVersion: state.LatestVersion, Checked: false}, nil
	}

	latest, err := updateCheckFetchLatestVersion(currentVersion)
	if err != nil {
		if hasState {
			return updateCheckResult{LatestVersion: state.LatestVersion, Checked: false}, nil
		}
		return updateCheckResult{}, err
	}
	state.LastCheckedAt = now.UTC()
	state.LatestVersion = latest
	_ = writeUpdateCheckState(statePath, state)
	return updateCheckResult{LatestVersion: latest, Checked: true}, nil
}

func resolveUpdateCheckStatePath() (string, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "cache", "update_check.json"), nil
}

func readUpdateCheckState(path string) (updateCheckState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return updateCheckState{}, false
	}
	var state updateCheckState
	if err := json.Unmarshal(data, &state); err != nil {
		return updateCheckState{}, false
	}
	state.LatestVersion = normalizeVersionTag(state.LatestVersion)
	return state, true
}

func writeUpdateCheckState(path string, state updateCheckState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fetchLatestVersionFromGitHub(currentVersion string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, updateCheckLatestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	userAgentVersion := currentVersion
	if userAgentVersion == "" {
		userAgentVersion = "dev"
	}
	req.Header.Set("User-Agent", "github.com/1024XEngineer/bytemind/"+userAgentVersion)

	resp, err := updateCheckHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected release check status: %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	latest := normalizeVersionTag(payload.TagName)
	if latest == "" {
		return "", fmt.Errorf("invalid release tag %q", payload.TagName)
	}
	return latest, nil
}

type semver struct {
	Major int
	Minor int
	Patch int
}

func normalizeVersionTag(version string) string {
	parsed, ok := parseVersion(version)
	if !ok {
		return ""
	}
	return fmt.Sprintf("v%d.%d.%d", parsed.Major, parsed.Minor, parsed.Patch)
}

func parseVersion(version string) (semver, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return semver{}, false
	}
	version = strings.TrimPrefix(version, "v")
	if cut := strings.IndexAny(version, "-+"); cut >= 0 {
		version = version[:cut]
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return semver{}, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return semver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return semver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return semver{}, false
	}

	return semver{Major: major, Minor: minor, Patch: patch}, true
}

func isVersionNewer(candidate, current string) bool {
	candidateVersion, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	currentVersion, ok := parseVersion(current)
	if !ok {
		return false
	}
	if candidateVersion.Major != currentVersion.Major {
		return candidateVersion.Major > currentVersion.Major
	}
	if candidateVersion.Minor != currentVersion.Minor {
		return candidateVersion.Minor > currentVersion.Minor
	}
	return candidateVersion.Patch > currentVersion.Patch
}
