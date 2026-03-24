package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerTimesOut(t *testing.T) {
	repo := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/slow\n\ngo 1.23.0\n",
		"slow_test.go": `package slow

import (
    "testing"
    "time"
)

func TestSlow(t *testing.T) {
    time.Sleep(250 * time.Millisecond)
}
`,
	}

	for name, content := range files {
		path := filepath.Join(repo, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runner := Runner{Timeout: 25 * time.Millisecond}
	result, err := runner.Run(context.Background(), repo, "go test ./...")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !result.TimedOut {
		t.Fatal("expected timed out result")
	}
}
