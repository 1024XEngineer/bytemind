package agent

import "regexp"

func init() {
	// Support compact clarify prompts such as:
	// "请选择一个方案：A（推荐）、B（Flask）、或 C（自定义）"
	clarifyChoiceShortcutPattern = regexp.MustCompile("(?i)(^|[\\s/|,;:\\-\u3001\uFF0C\uFF1A])([a-d]|1|2|3|4)([.)\\s:/,;\\-()\uFF08\uFF09\u3001\uFF0C\uFF1A]|$)")
}
