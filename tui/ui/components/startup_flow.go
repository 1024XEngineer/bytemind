package tui

import (
	"fmt"
	"strings"

	tuiruntime "bytemind/internal/tui/runtime"
)

func (m *model) handleStartupGuideSubmission(rawInput string) error {
	rawInput = strings.TrimSpace(rawInput)

	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	if explicitField, explicitValue, ok := parseStartupConfigInput(rawInput); ok {
		field = explicitField
		rawInput = explicitValue
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		value, err := m.resolveStartupFieldValue(field, rawInput)
		if err != nil {
			return err
		}
		if err := m.applyStartupConfigField(field, value); err != nil {
			return err
		}
		next := startupNextField(field)
		if next == "" {
			next = startupFieldAPIKey
		}
		m.setStartupGuideStep(next, "")
		m.input.Reset()
		return nil
	case startupFieldAPIKey:
		return m.verifyAndFinalizeStartupAPIKey(rawInput)
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}
}

func (m *model) verifyAndFinalizeStartupAPIKey(rawInput string) error {
	apiKey := sanitizeAPIKeyInput(rawInput)
	if apiKey == "" {
		return fmt.Errorf("please paste a non-empty API key")
	}

	result, err := m.runtimeAPI().VerifyStartupAPIKey(tuiruntime.StartupVerifyRequest{
		APIKey:     apiKey,
		ConfigPath: m.startupGuide.ConfigPath,
		Provider:   m.cfg.Provider,
	})
	if err != nil {
		return err
	}
	if !result.Ready {
		m.llmConnected = false
		m.phase = "error"
		m.setStartupGuideStep(startupFieldAPIKey, startupGuideIssueHint(result.Check))
		return nil
	}
	m.cfg.Provider = result.Provider
	m.startupGuide.ConfigPath = result.ConfigPath
	m.startupGuide.Active = false
	m.statusNote = "Provider configured and verified. You can start chatting."
	m.llmConnected = true
	m.phase = "idle"
	if result.SaveErr != nil {
		m.statusNote = "Provider verified, but config save failed: " + compact(result.SaveErr.Error(), 80)
	} else if strings.TrimSpace(result.WrittenPath) != "" {
		m.statusNote = "Provider configured and verified. Saved to " + compact(result.WrittenPath, 48)
	}
	m.syncInputStyle()
	m.input.Reset()
	return nil
}

func (m *model) applyStartupConfigField(field, value string) error {
	result, err := m.runtimeAPI().ApplyStartupField(tuiruntime.StartupFieldRequest{
		Field:      strings.TrimSpace(field),
		Value:      strings.TrimSpace(value),
		ConfigPath: m.startupGuide.ConfigPath,
		Provider:   m.cfg.Provider,
	})
	if err != nil {
		return err
	}
	m.cfg.Provider = result.Provider
	m.startupGuide.ConfigPath = result.ConfigPath
	return nil
}

func (m *model) resolveStartupFieldValue(field, rawInput string) (string, error) {
	value := strings.TrimSpace(rawInput)
	if value != "" {
		return value, nil
	}

	current := m.startupCurrentValue(field)
	if current != "" {
		return current, nil
	}

	switch field {
	case startupFieldType:
		return "openai-compatible", nil
	case startupFieldBaseURL:
		return startupProviderDefaultBaseURL(m.cfg.Provider.Type), nil
	case startupFieldModel:
		if fallback := startupProviderDefaultModel(m.cfg.Provider.Type); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("please enter model name for provider %s", strings.TrimSpace(m.cfg.Provider.Type))
	default:
		return "", fmt.Errorf("%s cannot be empty", startupFieldName(field))
	}
}

func (m *model) initializeStartupGuide() {
	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	m.setStartupGuideStep(field, "")
}

func (m *model) setStartupGuideStep(field, issue string) {
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	step, total := startupFieldStep(field)
	fieldName := startupFieldName(field)
	if strings.TrimSpace(issue) == "" {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s.", step, total, fieldName)
	} else {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s. %s", step, total, fieldName, issue)
	}
	m.statusNote = m.startupGuide.Status
	m.startupGuide.CurrentField = field
	m.startupGuide.Lines = startupGuideStepLines(field, m.cfg, m.startupGuide.ConfigPath, issue)
	m.syncInputStyle()
}
