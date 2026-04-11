package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func installFakeClaude(t *testing.T, projectRoot string) {
	t.Helper()

	binDir := filepath.Join(projectRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}

	scriptPath := filepath.Join(binDir, "claude")
	script := `#!/usr/bin/env bash
set -euo pipefail

mode=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "--output-format" ]]; then
    mode="$arg"
    break
  fi
  prev="$arg"
done

if [[ "$mode" == "text" ]]; then
  echo "<new_description>Improved description for tests</new_description>"
  exit 0
fi

if [[ "$mode" == "stream-json" ]]; then
  latest=$(ls -1 .claude/commands/*.md 2>/dev/null | tail -n 1 || true)
  name=""
  if [[ -n "$latest" ]]; then
    base=$(basename "$latest")
    name="${base%.md}"
  fi

  if [[ "${CODEX_FAKE_TRIGGER:-0}" == "1" ]]; then
    if [[ -n "$name" ]]; then
      printf '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"%s"}}]}}' "$name"
      printf '\n'
    else
      echo '{"type":"result"}'
    fi
    exit 0
  fi

  if [[ -n "$latest" ]] && grep -q "Improved description for tests" "$latest"; then
    printf '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"%s"}}]}}' "$name"
    printf '\n'
  else
    echo '{"type":"result"}'
  fi
  exit 0
fi

echo "unsupported invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	cmdPath := filepath.Join(binDir, "claude.cmd")
	cmdScript := `@echo off
setlocal EnableDelayedExpansion
set MODE=
set PREV=

:args
if "%~1"=="" goto afterArgs
if /I "!PREV!"=="--output-format" set MODE=%~1
set PREV=%~1
shift
goto args

:afterArgs
if /I "%MODE%"=="text" (
  echo ^<new_description^>Improved description for tests^</new_description^>
  exit /b 0
)

if /I "%MODE%"=="stream-json" (
  set CMD_DIR=.claude\commands
  set LATEST=
  for %%f in ("%CMD_DIR%\*.md") do set LATEST=%%f
  set NAME=
  if defined LATEST (
    for %%n in ("!LATEST!") do set NAME=%%~nn
  )

  if /I "%CODEX_FAKE_TRIGGER%"=="1" (
    if defined NAME (
      echo {"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"!NAME!"}}]}}
    ) else (
      echo {"type":"result"}
    )
    exit /b 0
  )

  if defined LATEST (
    findstr /C:"Improved description for tests" "!LATEST!" >nul
    if !errorlevel! EQU 0 (
      echo {"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"!NAME!"}}]}}
      exit /b 0
    )
  )

  echo {"type":"result"}
  exit /b 0
)

echo unsupported invocation>&2
exit /b 1
`
	if err := os.WriteFile(cmdPath, []byte(cmdScript), 0o755); err != nil {
		t.Fatalf("write fake claude cmd: %v", err)
	}

	pathEnv := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+pathEnv)
}

func createMinimalSkill(t *testing.T, root, name, desc string) string {
	t.Helper()
	skillDir := filepath.Join(root, name)
	skill := strings.Join([]string{
		"---",
		"name: " + name,
		"description: \"" + desc + "\"",
		"---",
		"",
		"# Skill Body",
		"",
	}, "\n")
	writeTestFile(t, filepath.Join(skillDir, "SKILL.md"), skill)
	return skillDir
}
