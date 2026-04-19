package runtime

import (
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
		Executed:               []string{"read_file"},
		Denied:                 []string{"write_file"},
		PendingApproval:        []string{"write_file"},
		SkippedDueToDependency: []string{"update_plan"},
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

func TestTaskReportRetryAndNoProgressAreNonSuccess(t *testing.T) {
	var report TaskReport
	report.RecordRetry("missing_structured_tool_call")
	report.RecordNoProgressTurn()
	report.RecordStrategyAdjustment("asked model to emit structured tool calls")
	report.RecordEscalation("semantic repair retries exceeded")

	if !report.HasNonSuccessOutcomes() {
		t.Fatal("expected retry/no-progress/escalation to mark non-success outcomes")
	}
	text := report.HumanSummary()
	for _, want := range []string{
		"- Retry reasons: missing_structured_tool_call",
		"- No progress turns: 1",
		"- Escalations: semantic repair retries exceeded",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected human summary to contain %q, got %q", want, text)
		}
	}
}
