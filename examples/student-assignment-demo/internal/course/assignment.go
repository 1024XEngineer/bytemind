package course

import "time"

type Assignment struct {
	ID     int
	Course string
	Title  string
	DueAt  time.Time
	Done   bool
}

func (a Assignment) IsOverdue(now time.Time) bool {
	return !a.Done && now.After(a.DueAt)
}
