package layout

import "testing"

func TestCandidateOriginsNear(t *testing.T) {
	got := CandidateOriginsNear(5, 10, 2)
	want := []int{5, 4, 6, 3, 7}
	if len(got) != len(want) {
		t.Fatalf("expected %d candidates, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected candidate order at %d: got=%d want=%d full=%v", i, got[i], want[i], got)
		}
	}

	if none := CandidateOriginsNear(0, -1, 2); none != nil {
		t.Fatalf("expected nil when maxOrigin<0, got %v", none)
	}
}

func TestRoundedScaledDivisionAndClamp(t *testing.T) {
	if got := RoundedScaledDivision(0, 10, 100); got != 0 {
		t.Fatalf("expected zero when value<=0, got %d", got)
	}
	if got := RoundedScaledDivision(1, 10, 3); got != 3 {
		t.Fatalf("expected rounded division result 3, got %d", got)
	}

	if got := Clamp(-1, 0, 10); got != 0 {
		t.Fatalf("expected clamp lower bound, got %d", got)
	}
	if got := Clamp(99, 0, 10); got != 10 {
		t.Fatalf("expected clamp upper bound, got %d", got)
	}
	if got := Clamp(6, 0, 10); got != 6 {
		t.Fatalf("expected in-range value unchanged, got %d", got)
	}
}
