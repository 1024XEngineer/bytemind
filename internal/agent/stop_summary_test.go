package agent

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecentUniqueToolNames(t *testing.T) {
	got := RecentUniqueToolNames([]string{"list_files", "read_file", "list_files", "search_text", "read_file"}, 3)
	want := []string{"list_files", "search_text", "read_file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected recent tool names: got=%v want=%v", got, want)
	}
}

func TestBuildStopSummaryIncludesReasonToolsAndSessionHint(t *testing.T) {
	summary := BuildStopSummary(StopSummaryInput{
		SessionID:     "sess-123",
		Reason:        "I reached the current execution budget.",
		ExecutedTools: []string{"list_files", "read_file", "list_files"},
	})

	for _, want := range []string{
		"Paused before a final answer.",
		"I reached the current execution budget.",
		"Recent tool activity:",
		"- read_file",
		"- list_files",
		"-session sess-123",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got %q", want, summary)
		}
	}
}

func TestBuildStopSummaryWithoutTools(t *testing.T) {
	summary := BuildStopSummary(StopSummaryInput{
		SessionID: "sess-empty",
		Reason:    "No tool calls were made.",
	})
	if strings.Contains(summary, "Recent tool activity:") {
		t.Fatalf("did not expect tool activity section, got %q", summary)
	}
}

func TestBuildStopSummaryIncludesTaskReportWhenPresent(t *testing.T) {
	report := &TaskReport{}
	report.RecordExecuted("read_file")
	report.RecordDenied("write_file")
	report.RecordPendingApproval("write_file")
	report.RecordSkippedDueToDependency("update_plan")

	summary := BuildStopSummary(StopSummaryInput{
		SessionID:         "sess-report",
		Reason:            "Stopped after permission denial.",
		TaskReport:        report,
		IncludeTaskReport: true,
	})
	for _, want := range []string{
		"Task report summary:",
		"- Pending approval: write_file",
		"- Skipped due to denied dependency: update_plan",
		"Task report (json):",
		`"executed":["read_file"]`,
		`"denied":["write_file"]`,
		`"pending_approval":["write_file"]`,
		`"skipped_due_to_denied_dependency":["update_plan"]`,
		`"skipped_due_to_dependency":["update_plan"]`,
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got %q", want, summary)
		}
	}
}

func TestBuildStopSummaryOmitsTaskReportByDefault(t *testing.T) {
	report := &TaskReport{}
	report.RecordDenied("write_file")

	summary := BuildStopSummary(StopSummaryInput{
		SessionID:  "sess-report",
		Reason:     "Stopped after permission denial.",
		TaskReport: report,
	})
	for _, unwanted := range []string{"Task report summary:", "Task report (json):"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("did not expect default stop summary to contain %q, got %q", unwanted, summary)
		}
	}
}
