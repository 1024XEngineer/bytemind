package course

import "strings"

func EmptySearchMessage(query string) string {
	if strings.TrimSpace(query) == "" {
		return "还没有作业，先新增一条吧。"
	}
	return "No assignments found."
}
