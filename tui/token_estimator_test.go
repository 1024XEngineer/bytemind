package tui

import "testing"

func TestEstimateDeltaTokensHandlesEmptyAndFallback(t *testing.T) {
	if got := estimateDeltaTokens(nil, "   "); got != 0 {
		t.Fatalf("expected empty text to estimate as zero, got %d", got)
	}

	// 8 runes => ceil(8/4) => 2
	if got := estimateDeltaTokens(nil, "abcdefgh"); got != 2 {
		t.Fatalf("expected fallback estimate 2, got %d", got)
	}
}

func TestRealtimeTokenEstimatorInitialization(t *testing.T) {
	est := newRealtimeTokenEstimator("")
	if est == nil || est.encoder == nil {
		t.Fatalf("expected default model estimator to initialize")
	}
	if got := estimateDeltaTokens(est, "hello token estimator"); got <= 0 {
		t.Fatalf("expected initialized estimator to return a positive token count, got %d", got)
	}

	// Invalid model name should still fall back to a base encoding.
	fallback := newRealtimeTokenEstimator("definitely-not-a-real-model-name")
	if fallback == nil || fallback.encoder == nil {
		t.Fatalf("expected invalid model to fall back to a base tokenizer")
	}
}

