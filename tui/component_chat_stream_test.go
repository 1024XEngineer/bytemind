package tui

import "testing"

func TestAppendAssistantDeltaStripsTurnIntentTag(t *testing.T) {
	m := model{}
	m.appendAssistantDelta("<turn_intent>finalize</turn_intent>已收到，开始执行")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected 1 chat item, got %d", len(m.chatItems))
	}
	if got := m.chatItems[0].Body; got != "已收到，开始执行" {
		t.Fatalf("expected cleaned body, got %q", got)
	}
}

