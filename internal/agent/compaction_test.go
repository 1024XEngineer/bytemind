package agent

import (
	"testing"

	"bytemind/internal/config"
)

func TestClassifyBudgetBoundaries(t *testing.T) {
	warning := config.DefaultContextBudgetWarningRatio
	critical := config.DefaultContextBudgetCriticalRatio

	tests := []struct {
		name     string
		usage    float64
		expected budgetLevel
	}{
		{name: "84.99 percent", usage: 0.8499, expected: budgetNone},
		{name: "85 percent", usage: 0.85, expected: budgetWarning},
		{name: "94.99 percent", usage: 0.9499, expected: budgetWarning},
		{name: "95 percent", usage: 0.95, expected: budgetCritical},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyBudget(tc.usage, warning, critical)
			if got != tc.expected {
				t.Fatalf("unexpected budget level: got=%q want=%q", got, tc.expected)
			}
		})
	}
}
