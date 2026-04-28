package tui

import "testing"

func TestInitReturnsBatchCommand(t *testing.T) {
	m := model{}
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("expected Init to return a non-nil command")
	}
}

func TestLandingGlowTickCmdEmitsLandingGlowTickMsg(t *testing.T) {
	cmd := landingGlowTickCmd()
	if cmd == nil {
		t.Fatalf("expected landing glow tick command to be non-nil")
	}
	if _, ok := cmd().(landingGlowTickMsg); !ok {
		t.Fatalf("expected landingGlowTickCmd to emit landingGlowTickMsg")
	}
}

func TestUpdateLandingGlowTickWrapsAndSchedulesNextTick(t *testing.T) {
	m := model{
		screen:          screenLanding,
		landingGlowStep: 2047,
	}

	updatedAny, cmd := m.Update(landingGlowTickMsg{})
	updated, ok := updatedAny.(model)
	if !ok {
		t.Fatalf("expected updated model type, got %T", updatedAny)
	}
	if updated.landingGlowStep != 0 {
		t.Fatalf("expected landingGlowStep to wrap to 0, got %d", updated.landingGlowStep)
	}
	if cmd == nil {
		t.Fatalf("expected Update to schedule next landing glow tick command")
	}
	if _, ok := cmd().(landingGlowTickMsg); !ok {
		t.Fatalf("expected scheduled command to emit landingGlowTickMsg")
	}
}

func TestUpdateLandingGlowTickStopsOutsideLanding(t *testing.T) {
	m := model{
		screen:          screenChat,
		landingGlowStep: 99,
	}

	updatedAny, cmd := m.Update(landingGlowTickMsg{})
	updated, ok := updatedAny.(model)
	if !ok {
		t.Fatalf("expected updated model type, got %T", updatedAny)
	}
	if updated.landingGlowStep != 99 {
		t.Fatalf("expected non-landing screen to keep glow step unchanged, got %d", updated.landingGlowStep)
	}
	if cmd != nil {
		t.Fatalf("expected non-landing screen to stop rescheduling glow tick")
	}
}

func TestStartLandingGlowOnTransition(t *testing.T) {
	m := model{screen: screenLanding}
	cmd := m.startLandingGlowOnTransition(screenChat)
	if cmd == nil {
		t.Fatalf("expected transition into landing to start glow tick")
	}
	if _, ok := cmd().(landingGlowTickMsg); !ok {
		t.Fatalf("expected transition command to emit landingGlowTickMsg")
	}

	if cmd := m.startLandingGlowOnTransition(screenLanding); cmd != nil {
		t.Fatalf("expected no glow tick when remaining on landing")
	}

	chat := model{screen: screenChat}
	if cmd := chat.startLandingGlowOnTransition(screenChat); cmd != nil {
		t.Fatalf("expected no glow tick when not entering landing")
	}
}
