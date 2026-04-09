package tui

import "testing"

func TestParseMouseCaptureEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: true},
		{name: "zero", value: "0", want: false},
		{name: "false", value: "false", want: false},
		{name: "no", value: "no", want: false},
		{name: "off", value: "off", want: false},
		{name: "random", value: "abc", want: true},
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

func TestParseInputTTYEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "zero", value: "0", want: false},
		{name: "false", value: "false", want: false},
		{name: "no", value: "no", want: false},
		{name: "off", value: "off", want: false},
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
			if got := parseInputTTYEnv(tc.value); got != tc.want {
				t.Fatalf("parseInputTTYEnv(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestDefaultAutoMouseYOffset(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		inputTTY    string
		wtSession   string
		termProgram string
		existing    string
		wantOffset  int
		wantApply   bool
	}{
		{
			name:       "windows terminal defaults to +2",
			goos:       "windows",
			inputTTY:   "0",
			wtSession:  "abc",
			existing:   "",
			wantOffset: 2,
			wantApply:  true,
		},
		{
			name:        "vscode terminal defaults to +2",
			goos:        "windows",
			inputTTY:    "0",
			termProgram: "vscode",
			existing:    "",
			wantOffset:  2,
			wantApply:   true,
		},
		{
			name:       "windows terminal input tty disables auto offset",
			goos:       "windows",
			inputTTY:   "1",
			wtSession:  "abc",
			existing:   "",
			wantOffset: 0,
			wantApply:  false,
		},
		{
			name:       "non windows does not auto offset",
			goos:       "linux",
			inputTTY:   "0",
			wtSession:  "abc",
			existing:   "",
			wantOffset: 0,
			wantApply:  false,
		},
		{
			name:       "explicit existing offset wins",
			goos:       "windows",
			inputTTY:   "0",
			wtSession:  "abc",
			existing:   "3",
			wantOffset: 0,
			wantApply:  false,
		},
		{
			name:       "windows unknown host does not auto offset",
			goos:       "windows",
			inputTTY:   "0",
			existing:   "",
			wantOffset: 0,
			wantApply:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotOffset, gotApply := defaultAutoMouseYOffset(tc.goos, tc.inputTTY, tc.wtSession, tc.termProgram, tc.existing)
			if gotApply != tc.wantApply || gotOffset != tc.wantOffset {
				t.Fatalf(
					"defaultAutoMouseYOffset(%q, %q, %q, %q, %q) = (%d, %v), want (%d, %v)",
					tc.goos, tc.inputTTY, tc.wtSession, tc.termProgram, tc.existing,
					gotOffset, gotApply, tc.wantOffset, tc.wantApply,
				)
			}
		})
	}
}
