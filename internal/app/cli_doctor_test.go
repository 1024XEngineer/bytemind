package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDoctorOutputContainsExpectedSections(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	sections := []string{
		"ByteMind Doctor",
		"Configuration:",
		"Workspace:",
		"Security:",
		"Environment:",
	}
	for _, section := range sections {
		if !strings.Contains(output, section) {
			t.Errorf("expected output to contain %q", section)
		}
	}
}

func TestRunDoctorWithWorkspaceFlag(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{"-workspace", "."}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "workspace") {
		t.Errorf("expected workspace-related output, got %s", output)
	}
}

func TestRunDoctorOutputContainsToolCount(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "tools registered") {
		t.Errorf("expected tool count in output, got %s", output)
	}
}

func TestRunDoctorOutputContainsGoVersion(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Go ") {
		t.Errorf("expected Go version in output, got %s", output)
	}
}

func TestRunDoctorOutputContainsEmoji(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "\u2705") {
		t.Errorf("expected checkmark emoji in output")
	}
}

func TestRunDoctorWithConfigFlag(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{"-config", "nonexistent.json"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Doctor check complete") {
		t.Errorf("expected doctor to complete, got %s", output[:100])
	}
}

func TestRunDoctorUnknownFlagIgnored(t *testing.T) {
	var stdout bytes.Buffer
	err := RunDoctor([]string{"-unknown-flag"}, &stdout, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "ByteMind Doctor") {
		t.Errorf("expected doctor output for unknown flag, got %s", output[:100])
	}
}
