package course

import "strings"

func SearchAssignments(assignments []Assignment, query string) []Assignment {
	query = strings.TrimSpace(query)
	if query == "" {
		return append([]Assignment(nil), assignments...)
	}

	var matches []Assignment
	for _, assignment := range assignments {
		if strings.Contains(assignment.Title, query) {
			matches = append(matches, assignment)
		}
	}
	return matches
}
