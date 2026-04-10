package dialog

import "testing"

func TestParseStartupConfigInputAliases(t *testing.T) {
	tests := []struct {
		in    string
		field string
		value string
		ok    bool
	}{
		{"model=GPT-5.4", "model", "GPT-5.4", true},
		{"base-url:https://api.example.com", "base_url", "https://api.example.com", true},
		{"provider:anthropic", "type", "anthropic", true},
		{"apikey=sk-test", "api_key", "sk-test", true},
		{"", "", "", false},
	}
	for _, tc := range tests {
		field, value, ok := ParseStartupConfigInput(tc.in)
		if field != tc.field || value != tc.value || ok != tc.ok {
			t.Fatalf("unexpected parse result for %q: field=%q value=%q ok=%v", tc.in, field, value, ok)
		}
	}
}

func TestSanitizeAPIKeyInput(t *testing.T) {
	cases := map[string]string{
		`"sk-quoted"`:                     "sk-quoted",
		"Bearer sk-token":                 "sk-token",
		"Authorization: Bearer sk-token2": "sk-token2",
	}
	for in, want := range cases {
		if got := SanitizeAPIKeyInput(in); got != want {
			t.Fatalf("expected %q -> %q, got %q", in, want, got)
		}
	}
}

func TestNormalizeStartupProviderType(t *testing.T) {
	if got, ok := NormalizeStartupProviderType("openai"); !ok || got != "openai-compatible" {
		t.Fatalf("expected openai alias to normalize, got=%q ok=%v", got, ok)
	}
	if got, ok := NormalizeStartupProviderType("anthropic"); !ok || got != "anthropic" {
		t.Fatalf("expected anthropic to normalize, got=%q ok=%v", got, ok)
	}
	if _, ok := NormalizeStartupProviderType("other"); ok {
		t.Fatalf("expected unknown provider to fail normalization")
	}
}

func TestParseSkillArgs(t *testing.T) {
	args, err := ParseSkillArgs([]string{"lang=go", "mode=fast"})
	if err != nil {
		t.Fatalf("expected args parse success, got err=%v", err)
	}
	if args["lang"] != "go" || args["mode"] != "fast" {
		t.Fatalf("unexpected parsed args: %#v", args)
	}

	if _, err := ParseSkillArgs([]string{"bad"}); err == nil {
		t.Fatalf("expected invalid arg format to fail")
	}
	if _, err := ParseSkillArgs([]string{"k="}); err == nil {
		t.Fatalf("expected empty value to fail")
	}
}

func TestParsePromptSearchQuery(t *testing.T) {
	tokens, ws, sid := ParsePromptSearchQuery("fix bug ws:repo sid:1234")
	if ws != "repo" || sid != "1234" {
		t.Fatalf("expected filters parsed, got ws=%q sid=%q", ws, sid)
	}
	if len(tokens) != 2 || tokens[0] != "fix" || tokens[1] != "bug" {
		t.Fatalf("expected prompt tokens kept, got %#v", tokens)
	}
}
