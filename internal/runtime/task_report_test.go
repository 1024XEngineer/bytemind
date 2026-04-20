package runtime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskReportRecordsUniqueItemsInOrder(t *testing.T) {
	var report TaskReport
	report.RecordExecuted("list_files")
	report.RecordExecuted("read_file")
	report.RecordExecuted("list_files")
	report.RecordDenied("write_file")
	report.RecordPendingApproval("write_file")
	report.RecordPendingApproval("write_file")
	report.RecordSkippedDueToDependency("update_plan")

	if got, want := strings.Join(report.Executed, ","), "list_files,read_file"; got != want {
		t.Fatalf("unexpected executed tools: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(report.Denied, ","), "write_file"; got != want {
		t.Fatalf("unexpected denied tools: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(report.PendingApproval, ","), "write_file"; got != want {
		t.Fatalf("unexpected pending approval tools: got=%q want=%q", got, want)
	}
	if got, want := strings.Join(report.SkippedDueToDependency, ","), "update_plan"; got != want {
		t.Fatalf("unexpected skipped tools: got=%q want=%q", got, want)
	}
}

func TestTaskReportJSONAndEmpty(t *testing.T) {
	var report TaskReport
	if !report.IsEmpty() {
		t.Fatal("expected zero-value report to be empty")
	}
	if report.HasNonSuccessOutcomes() {
		t.Fatal("expected zero-value report to have no non-success outcomes")
	}
	if got := report.JSON(); got != "{}" {
		t.Fatalf("expected empty JSON object, got %q", got)
	}

	report.RecordExecuted("run_shell")
	if report.IsEmpty() {
		t.Fatal("expected non-empty report after recording an entry")
	}
	if report.HasNonSuccessOutcomes() {
		t.Fatal("did not expect executed-only report to mark non-success outcomes")
	}
	if got := report.JSON(); !strings.Contains(got, `"executed":["run_shell"]`) {
		t.Fatalf("expected executed payload in JSON, got %q", got)
	}
}

func TestTaskReportHumanSummaryIncludesPendingApproval(t *testing.T) {
	report := TaskReport{
		Executed:                     []string{"read_file"},
		Denied:                       []string{"write_file"},
		PendingApproval:              []string{"write_file"},
		SkippedDueToDeniedDependency: []string{"update_plan"},
	}
	text := report.HumanSummary()
	for _, want := range []string{
		"- Executed: read_file",
		"- Denied: write_file",
		"- Pending approval: write_file",
		"- Skipped due to denied dependency: update_plan",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected human summary to contain %q, got %q", want, text)
		}
	}
	if !report.HasNonSuccessOutcomes() {
		t.Fatal("expected report with denied/pending/skipped entries to mark non-success outcomes")
	}
}

func TestTaskReportJSONIncludesBothSkippedFieldsForCompatibility(t *testing.T) {
	var report TaskReport
	report.RecordSkippedDueToDeniedDependency("read_file")
	got := report.JSON()
	for _, want := range []string{
		`"skipped_due_to_denied_dependency":["read_file"]`,
		`"skipped_due_to_dependency":["read_file"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in JSON payload, got %q", want, got)
		}
	}
}

func TestTaskReportUnmarshalLegacySkippedFieldIntoNewField(t *testing.T) {
	var report TaskReport
	if err := json.Unmarshal([]byte(`{"skipped_due_to_dependency":["update_plan"]}`), &report); err != nil {
		t.Fatalf("unmarshal legacy report: %v", err)
	}
	if got, want := strings.Join(report.SkippedDueToDeniedDependency, ","), "update_plan"; got != want {
		t.Fatalf("expected legacy skipped field to map into new field, got=%q want=%q", got, want)
	}
	if got, want := strings.Join(report.SkippedDueToDependency, ","), "update_plan"; got != want {
		t.Fatalf("expected legacy skipped field to be preserved, got=%q want=%q", got, want)
	}
}
