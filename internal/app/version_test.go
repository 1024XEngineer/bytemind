package app

import "testing"

func withAppVersion(t *testing.T, version, gitVersion string) {
	t.Helper()

	previousBuildVersion := buildVersion
	previousGitDescribe := gitDescribe
	buildVersion = version
	gitDescribe = func() string { return gitVersion }
	t.Cleanup(func() {
		buildVersion = previousBuildVersion
		gitDescribe = previousGitDescribe
	})
}

func TestCurrentVersionUsesInjectedBuildVersionWhenProvided(t *testing.T) {
	withAppVersion(t, "v1.2.3", "v9.9.9")

	if got := CurrentVersion(); got != "v1.2.3" {
		t.Fatalf("expected injected build version, got %q", got)
	}
}

func TestCurrentVersionFallsBackToGitDescribeWhenBuildVersionIsDev(t *testing.T) {
	withAppVersion(t, "dev", "v2.0.1-3-gabc123")

	if got := CurrentVersion(); got != "v2.0.1-3-gabc123" {
		t.Fatalf("expected git describe fallback, got %q", got)
	}
}

func TestCurrentVersionReturnsDevWhenNoInjectedOrGitVersion(t *testing.T) {
	withAppVersion(t, "dev", "")

	if got := CurrentVersion(); got != "dev" {
		t.Fatalf("expected dev fallback, got %q", got)
	}
}
