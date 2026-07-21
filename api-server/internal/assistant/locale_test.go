package assistant

import (
	"strings"
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: LocaleEnUS, want: LocaleEnUS},
		{input: "  en-US  ", want: LocaleEnUS},
		{input: "en-US,en;q=0.9,zh-CN;q=0.8", want: LocaleEnUS},
		{input: "en", want: LocaleEnUS},
		{input: LocaleZhCN, want: LocaleZhCN},
		{input: "fr-FR", want: LocaleZhCN},
		{input: "", want: LocaleZhCN},
	}
	for _, test := range tests {
		if got := NormalizeLocale(test.input); got != test.want {
			t.Fatalf("NormalizeLocale(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestResponseLanguageInstruction(t *testing.T) {
	if got := responseLanguageInstruction(LocaleEnUS); !strings.Contains(got, "in English") {
		t.Fatalf("English instruction missing language requirement: %q", got)
	}
	if got := responseLanguageInstruction(LocaleZhCN); !strings.Contains(got, "Simplified Chinese") {
		t.Fatalf("Chinese instruction missing language requirement: %q", got)
	}
}

func TestLocalized(t *testing.T) {
	if got := localized(LocaleEnUS, "中文", "English"); got != "English" {
		t.Fatalf("localized English = %q", got)
	}
	if got := localized(LocaleZhCN, "中文", "English"); got != "中文" {
		t.Fatalf("localized Chinese = %q", got)
	}
}
