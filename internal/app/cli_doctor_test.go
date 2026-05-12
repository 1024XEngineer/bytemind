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
