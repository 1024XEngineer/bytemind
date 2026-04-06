package tui

import "testing"

func TestParseMouseCaptureEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "zero", value: "0", want: false},
		{name: "false", value: "false", want: false},
		{name: "random", value: "abc", want: false},
		{name: "one", value: "1", want: true},
		{name: "true", value: "true", want: true},
		{name: "upper true", value: "TRUE", want: true},
		{name: "yes", value: "yes", want: true},
		{name: "on with spaces", value: " on ", want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := parseMouseCaptureEnv(tc.value); got != tc.want {
				t.Fatalf("parseMouseCaptureEnv(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
