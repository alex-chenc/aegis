package assistant

import "strings"

const (
	LocaleZhCN = "zh-CN"
	LocaleEnUS = "en-US"
)

func NormalizeLocale(value string) string {
	primary := strings.ToLower(strings.TrimSpace(strings.Split(value, ",")[0]))
	primary = strings.TrimSpace(strings.Split(primary, ";")[0])
	switch {
	case primary == "en" || strings.HasPrefix(primary, "en-"):
		return LocaleEnUS
	default:
		return LocaleZhCN
	}
}

func localized(locale, zhCN, enUS string) string {
	if NormalizeLocale(locale) == LocaleEnUS {
		return enUS
	}
	return zhCN
}

func responseLanguageInstruction(locale string) string {
	if NormalizeLocale(locale) == LocaleEnUS {
		return "Write all user-facing natural-language fields in English unless the user explicitly requests another language. Keep tool names, identifiers, arguments, enum values, paths, commands, and evidence unchanged."
	}
	return "Write all user-facing natural-language fields in Simplified Chinese unless the user explicitly requests another language. Keep tool names, identifiers, arguments, enum values, paths, commands, and evidence unchanged."
}
