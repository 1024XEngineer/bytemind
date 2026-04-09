package mention

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceFileIndexNilSafety(t *testing.T) {
	var idx *WorkspaceFileIndex
	if got := idx.Search("", 0); got != nil {
		t.Fatalf("expected nil search result for nil index, got %#v", got)
	}
	stats := idx.Stats()
	if stats.Count != 0 || stats.MaxFiles != 0 || stats.Truncated || stats.Ready {
		t.Fatalf("expected zero stats for nil index, got %#v", stats)
	}
}

func TestNewStaticWorkspaceFileIndexNormalizesCandidates(t *testing.T) {
	osPath := filepath.Join("internal", "tui", "model.go")
	idx := NewStaticWorkspaceFileIndex([]Candidate{
		{Path: " " + osPath + " "},
		{Path: ""},
		{Path: "README.md", BaseName: "README.md", TypeTag: "md"},
	}, 0, true)

	stats := idx.Stats()
	if !stats.Ready {
		t.Fatalf("expected static index to be ready")
	}
	if !stats.Truncated {
		t.Fatalf("expected static index to preserve truncated flag")
	}
	if stats.MaxFiles != 2 {
		t.Fatalf("expected max files to default to valid candidate count, got %d", stats.MaxFiles)
	}

	results := idx.Search("", 10)
	if len(results) != 2 {
		t.Fatalf("expected two valid candidates, got %d", len(results))
	}
	if results[0].Path != "README.md" || results[0].BaseName != "README.md" || results[0].TypeTag != "md" {
		t.Fatalf("unexpected first candidate: %#v", results[0])
	}
	if results[1].Path != "internal/tui/model.go" {
		t.Fatalf("expected normalized slash path, got %#v", results[1])
	}
	if results[1].BaseName != "model.go" || results[1].TypeTag != "go" {
		t.Fatalf("expected basename/tag to be auto-filled, got %#v", results[1])
	}
}

func TestFindActiveToken(t *testing.T) {
	t.Run("detects trailing mention", func(t *testing.T) {
		token, ok := FindActiveToken("please check @model")
		if !ok {
			t.Fatalf("expected trailing mention to be detected")
		}
		if token.Query != "model" {
			t.Fatalf("expected query model, got %q", token.Query)
		}
	})

	t.Run("supports empty query", func(t *testing.T) {
		token, ok := FindActiveToken("@")
		if !ok {
			t.Fatalf("expected single @ to open mention mode")
		}
		if token.Query != "" {
			t.Fatalf("expected empty mention query, got %q", token.Query)
		}
	})

	t.Run("ignores whitespace tail", func(t *testing.T) {
		if _, ok := FindActiveToken("@model "); ok {
			t.Fatalf("did not expect mention detection when trailing whitespace exists")
		}
	})

	t.Run("ignores email-like token", func(t *testing.T) {
		if _, ok := FindActiveToken("mail a@b.com"); ok {
			t.Fatalf("did not expect mention detection for email token")
		}
	})
}

func TestInsertIntoInput(t *testing.T) {
	token := Token{
		Query: "mod",
		Start: len([]rune("open ")),
		End:   len([]rune("open @mod")),
	}
	got := InsertIntoInput("open @mod", token, "internal/tui/model.go")
	want := "open @internal/tui/model.go "
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInsertIntoInputHandlesMidTokenAndInvalidRange(t *testing.T) {
	t.Run("insert before non-space tail", func(t *testing.T) {
		input := "open @mod,now"
		token := Token{
			Query: "mod",
			Start: len([]rune("open ")),
			End:   len([]rune("open @mod")),
		}
		got := InsertIntoInput(input, token, "internal/tui/model.go")
		want := "open @internal/tui/model.go ,now"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("invalid token range keeps original", func(t *testing.T) {
		input := "open @mod"
		token := Token{Start: -1, End: 3}
		got := InsertIntoInput(input, token, "x.go")
		if got != input {
			t.Fatalf("expected input to remain unchanged, got %q", got)
		}
	})
}

func TestWorkspaceFileIndexSearchSkipsIgnoredDirectories(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "README.md"), "hello")
	mustWriteMentionFile(t, filepath.Join(workspace, "internal", "tui", "model.go"), "package mention")
	mustWriteMentionFile(t, filepath.Join(workspace, ".git", "config"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "node_modules", "pkg", "index.js"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "vendor", "pkg", "a.go"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "dist", "bundle.js"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "build", "artifact.txt"), "ignored")

	index := NewWorkspaceFileIndex(workspace)
	all := index.Search("", 100)
	paths := make([]string, 0, len(all))
	for _, item := range all {
		paths = append(paths, item.Path)
	}
	for _, want := range []string{"README.md", "internal/tui/model.go"} {
		if !containsString(paths, want) {
			t.Fatalf("expected indexed files to include %q, got %v", want, paths)
		}
	}
	for _, unwanted := range []string{
		".git/config",
		"node_modules/pkg/index.js",
		"vendor/pkg/a.go",
		"dist/bundle.js",
		"build/artifact.txt",
	} {
		if containsString(paths, unwanted) {
			t.Fatalf("did not expect indexed files to include %q", unwanted)
		}
	}

	filtered := index.Search("model", 5)
	if len(filtered) == 0 {
		t.Fatalf("expected mention search to return matches for model")
	}
	if filtered[0].Path != "internal/tui/model.go" {
		t.Fatalf("expected best match to be internal/tui/model.go, got %q", filtered[0].Path)
	}
	if filtered[0].TypeTag != "go" {
		t.Fatalf("expected model.go tag to be go, got %q", filtered[0].TypeTag)
	}
}

func TestWorkspaceFileIndexSearchWithRecencyPrioritizesRecent(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "alpha.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "beta.go"), "package main")

	index := NewWorkspaceFileIndex(workspace)
	results := index.SearchWithRecency("", 10, map[string]int{
		"beta.go":  20,
		"alpha.go": 1,
	})
	if len(results) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(results))
	}
	if results[0].Path != "beta.go" {
		t.Fatalf("expected recent file beta.go first, got %q", results[0].Path)
	}
}

func TestWorkspaceFileIndexUsesTriePrefixRecall(t *testing.T) {
	idx := NewStaticWorkspaceFileIndex([]Candidate{
		{Path: "README.md"},
		{Path: "internal/tui/model.go"},
		{Path: "internal/tui/module.go"},
	}, 0, false)

	if idx.trie == nil {
		t.Fatal("expected trie to be initialized")
	}
	prefixHits := idx.trie.prefixIndices("rea", 5)
	if len(prefixHits) == 0 {
		t.Fatal("expected trie prefix recall to return hits")
	}
	if idx.files[prefixHits[0]].Path != "README.md" {
		t.Fatalf("expected README.md to be first trie prefix hit, got %q", idx.files[prefixHits[0]].Path)
	}

	results := idx.Search("rea", 5)
	if len(results) == 0 || results[0].Path != "README.md" {
		t.Fatalf("expected README.md to rank first for prefix query, got %#v", results)
	}
}

func TestWorkspaceFileIndexSupportsFuzzyAbbreviation(t *testing.T) {
	idx := NewStaticWorkspaceFileIndex([]Candidate{
		{Path: "internal/tui/model.go"},
		{Path: "internal/tui/module.go"},
		{Path: "internal/tui/monitor.go"},
	}, 0, false)

	results := idx.Search("modl", 5)
	if len(results) == 0 {
		t.Fatal("expected fuzzy abbreviation to return matches")
	}
	if results[0].Path != "internal/tui/model.go" {
		t.Fatalf("expected model.go to rank first for modl, got %q", results[0].Path)
	}
}

func TestWorkspaceFileIndexSupportsConfigurableIgnoreRules(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "keep.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "envskip.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "logs", "debug.log"), "line")
	mustWriteMentionFile(t, filepath.Join(workspace, "custom", "skip.txt"), "line")
	mustWriteMentionFile(t, filepath.Join(workspace, ".bytemindignore"), "custom/*\n")
	t.Setenv("BYTEMIND_MENTION_IGNORE", "envskip.go,logs/*")

	index := NewWorkspaceFileIndex(workspace)
	results := index.Search("", 50)
	paths := make([]string, 0, len(results))
	for _, item := range results {
		paths = append(paths, item.Path)
	}

	if !containsString(paths, "keep.go") {
		t.Fatalf("expected keep.go in results, got %v", paths)
	}
	for _, unwanted := range []string{"envskip.go", "logs/debug.log", "custom/skip.txt"} {
		if containsString(paths, unwanted) {
			t.Fatalf("did not expect ignored path %q in results %v", unwanted, paths)
		}
	}
}

func TestWorkspaceFileIndexRespectsMaxFilesLimitFromEnv(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "a.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "b.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "c.go"), "package main")
	t.Setenv("BYTEMIND_MENTION_MAX_FILES", "2")

	index := NewWorkspaceFileIndex(workspace)
	results := index.Search("", 50)
	if len(results) != 2 {
		t.Fatalf("expected max-files limited result count 2, got %d", len(results))
	}
	stats := index.Stats()
	if !stats.Truncated {
		t.Fatalf("expected stats to mark index as truncated")
	}
	if stats.MaxFiles != 2 {
		t.Fatalf("expected max files 2 from env, got %d", stats.MaxFiles)
	}
}

func TestWorkspaceFileIndexRebuildsAfterRefreshInterval(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "a.txt"), "a")
	index := NewWorkspaceFileIndex(workspace)
	first := index.Search("", 10)
	if len(first) != 1 {
		t.Fatalf("expected initial index size 1, got %d", len(first))
	}

	mustWriteMentionFile(t, filepath.Join(workspace, "b.txt"), "b")
	index.mu.Lock()
	index.lastBuild = time.Now().Add(-mentionIndexRefreshInterval - time.Second)
	index.mu.Unlock()

	deadline := time.Now().Add(800 * time.Millisecond)
	for {
		second := index.Search("", 10)
		if len(second) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected rebuilt index size 2, got %d", len(second))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMentionTypeTag(t *testing.T) {
	cases := map[string]string{
		"main.go":        "go",
		"README.md":      "md",
		"script.ps1":     "ps1",
		"archive.tar.gz": "gz",
		"noext":          "file",
	}
	for input, want := range cases {
		if got := mentionTypeTag(input); got != want {
			t.Fatalf("expected mentionTypeTag(%q)=%q, got %q", input, want, got)
		}
	}
}

func TestMentionMaxFilesFromEnvFallbacks(t *testing.T) {
	t.Setenv("BYTEMIND_MENTION_MAX_FILES", "abc")
	if got := mentionMaxFilesFromEnv(); got != mentionIndexDefaultMaxFiles {
		t.Fatalf("expected invalid env to use default max files, got %d", got)
	}

	t.Setenv("BYTEMIND_MENTION_MAX_FILES", "0")
	if got := mentionMaxFilesFromEnv(); got != mentionIndexDefaultMaxFiles {
		t.Fatalf("expected non-positive env to use default max files, got %d", got)
	}
}

func TestMentionIgnoreMatcherCoversExactAndGlob(t *testing.T) {
	matcher := mentionIgnoreMatcher{
		exact: map[string]struct{}{
			"secret.txt":      {},
			"docs/private.md": {},
		},
		globs: []string{"logs/*", "*.tmp"},
	}

	if !matcher.SkipFile("secret.txt", "secret.txt") {
		t.Fatalf("expected exact name rule to match")
	}
	if !matcher.SkipFile("private.md", "docs/private.md") {
		t.Fatalf("expected exact path rule to match")
	}
	if !matcher.SkipFile("error.log", "logs/error.log") {
		t.Fatalf("expected glob path rule to match")
	}
	if !matcher.SkipFile("cache.tmp", "cache.tmp") {
		t.Fatalf("expected glob name rule to match")
	}
	if matcher.SkipFile("README.md", "README.md") {
		t.Fatalf("did not expect non-matching file to be skipped")
	}
	if matcher.SkipFile("", "") {
		t.Fatalf("did not expect empty values to be skipped")
	}
}

func mustWriteMentionFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
