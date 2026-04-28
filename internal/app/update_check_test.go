package app

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
)

func TestMaybePrintUpdateReminderSkipsWhenDisabled(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	cfg := config.Default(t.TempDir())
	cfg.UpdateCheck.Enabled = false

	withAppVersion(t, "v1.0.0", "")

	previousFetcher := updateCheckFetchLatestVersion
	calls := 0
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		calls++
		return "v1.1.0", nil
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	var output bytes.Buffer
	maybePrintUpdateReminder(cfg, &output)

	if calls != 0 {
		t.Fatalf("expected disabled update check to skip fetch, got %d calls", calls)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no output when update check disabled, got %q", output.String())
	}
}

func TestMaybePrintUpdateReminderChecksAtMostOncePerDay(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	cfg := config.Default(t.TempDir())
	cfg.UpdateCheck.Enabled = true

	withAppVersion(t, "v1.0.0", "")

	previousNow := updateCheckNow
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	updateCheckNow = func() time.Time { return now }
	defer func() { updateCheckNow = previousNow }()

	previousFetcher := updateCheckFetchLatestVersion
	calls := 0
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		calls++
		return "v1.1.0", nil
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	var first bytes.Buffer
	maybePrintUpdateReminder(cfg, &first)
	if calls != 1 {
		t.Fatalf("expected first run to fetch once, got %d", calls)
	}
	if first.Len() == 0 {
		t.Fatal("expected first run to print update reminder")
	}

	var second bytes.Buffer
	maybePrintUpdateReminder(cfg, &second)
	if calls != 1 {
		t.Fatalf("expected second run within cache window not to re-fetch, got %d", calls)
	}
	if second.Len() != 0 {
		t.Fatalf("expected no second reminder within cache window, got %q", second.String())
	}

	now = now.Add(25 * time.Hour)
	var third bytes.Buffer
	maybePrintUpdateReminder(cfg, &third)
	if calls != 2 {
		t.Fatalf("expected reminder to re-check after cache window, got %d fetches", calls)
	}
	if third.Len() == 0 {
		t.Fatal("expected reminder after cache window")
	}
}

func TestMaybePrintUpdateReminderSkipsDevVersion(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	cfg := config.Default(t.TempDir())
	cfg.UpdateCheck.Enabled = true

	withAppVersion(t, "dev", "")

	previousFetcher := updateCheckFetchLatestVersion
	calls := 0
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		calls++
		return "v1.1.0", nil
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	var output bytes.Buffer
	maybePrintUpdateReminder(cfg, &output)
	if calls != 0 {
		t.Fatalf("expected dev version to skip update fetch, got %d calls", calls)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no output for dev version, got %q", output.String())
	}
}

func TestMaybePrintUpdateReminderSkipsWhenVersionNotNewer(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	cfg := config.Default(t.TempDir())
	cfg.UpdateCheck.Enabled = true

	withAppVersion(t, "v1.2.0", "")

	previousNow := updateCheckNow
	updateCheckNow = func() time.Time { return time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC) }
	defer func() { updateCheckNow = previousNow }()

	previousFetcher := updateCheckFetchLatestVersion
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		return "v1.2.0", nil
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	var output bytes.Buffer
	maybePrintUpdateReminder(cfg, &output)
	if output.Len() != 0 {
		t.Fatalf("expected no output when latest version is not newer, got %q", output.String())
	}
}

func TestLatestVersionWithCacheFallsBackToCachedOnFetchFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)

	statePath := filepath.Join(home, "cache", "update_check.json")
	state := updateCheckState{
		LastCheckedAt: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
		LatestVersion: "v1.2.3",
	}
	if err := writeUpdateCheckState(statePath, state); err != nil {
		t.Fatalf("write cache state: %v", err)
	}

	previousFetcher := updateCheckFetchLatestVersion
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		return "", errors.New("network down")
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	result, err := latestVersionWithCache(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC), "v1.0.0")
	if err != nil {
		t.Fatalf("expected cached fallback without error, got %v", err)
	}
	if result.Checked {
		t.Fatalf("expected checked=false when falling back to cache, got %#v", result)
	}
	if result.LatestVersion != "v1.2.3" {
		t.Fatalf("expected cached version v1.2.3, got %q", result.LatestVersion)
	}
}

func TestLatestVersionWithCacheReturnsErrorWhenNoCacheAndFetchFails(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	previousFetcher := updateCheckFetchLatestVersion
	updateCheckFetchLatestVersion = func(currentVersion string) (string, error) {
		return "", errors.New("network down")
	}
	defer func() { updateCheckFetchLatestVersion = previousFetcher }()

	_, err := latestVersionWithCache(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC), "v1.0.0")
	if err == nil {
		t.Fatal("expected error when no cached state exists and fetch fails")
	}
}

func TestReadUpdateCheckStateReturnsFalseOnInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_check.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid state: %v", err)
	}

	_, ok := readUpdateCheckState(path)
	if ok {
		t.Fatal("expected invalid json state to be ignored")
	}
}

func TestFetchLatestVersionFromGitHubSuccessAndHeaders(t *testing.T) {
	receivedAccept := ""
	receivedUserAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccept = r.Header.Get("Accept")
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.4.2"}`))
	}))
	defer server.Close()

	previousURL := updateCheckLatestReleaseURL
	previousClient := updateCheckHTTPClient
	updateCheckLatestReleaseURL = server.URL
	updateCheckHTTPClient = server.Client()
	defer func() {
		updateCheckLatestReleaseURL = previousURL
		updateCheckHTTPClient = previousClient
	}()

	latest, err := fetchLatestVersionFromGitHub("v1.0.0")
	if err != nil {
		t.Fatalf("expected successful fetch, got %v", err)
	}
	if latest != "v1.4.2" {
		t.Fatalf("expected normalized latest version v1.4.2, got %q", latest)
	}
	if receivedAccept != "application/vnd.github+json" {
		t.Fatalf("expected accept header, got %q", receivedAccept)
	}
	if receivedUserAgent != "github.com/1024XEngineer/bytemind/v1.0.0" {
		t.Fatalf("expected user-agent header, got %q", receivedUserAgent)
	}
}

func TestFetchLatestVersionFromGitHubErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{
			name:       "non-200 status",
			statusCode: http.StatusInternalServerError,
			body:       `{"tag_name":"v1.0.0"}`,
			wantErr:    "unexpected release check status",
		},
		{
			name:       "invalid tag",
			statusCode: http.StatusOK,
			body:       `{"tag_name":"not-a-semver-tag"}`,
			wantErr:    "invalid release tag",
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			body:       `{bad`,
			wantErr:    "invalid character",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			previousURL := updateCheckLatestReleaseURL
			previousClient := updateCheckHTTPClient
			updateCheckLatestReleaseURL = server.URL
			updateCheckHTTPClient = server.Client()
			defer func() {
				updateCheckLatestReleaseURL = previousURL
				updateCheckHTTPClient = previousClient
			}()

			_, err := fetchLatestVersionFromGitHub("v1.0.0")
			if err == nil {
				t.Fatal("expected fetch error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
