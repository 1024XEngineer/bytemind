package tui

import planpkg "bytemind/internal/plan"

func (m *model) toggleMode() {
	if m.mode == modeBuild {
		m.mode = modePlan
		if m.plan.Phase == planpkg.PhaseNone {
			m.plan.Phase = planpkg.PhaseDrafting
		}
		m.statusNote = "Switched to Plan mode. Draft the plan before executing."
	} else {
		m.mode = modeBuild
		m.statusNote = "Switched to Build mode. Execution still requires confirmation."
	}
	if m.sess != nil {
		m.sess.Mode = planpkg.NormalizeMode(string(m.mode))
		m.sess.Plan = copyPlanState(m.plan)
		_ = m.runtimeAPI().SaveSession(m.sess)
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}
