package main

import (
	"fmt"
	"time"

	"github.com/bytemind-demo/coursehub/internal/course"
)

func main() {
	assignments := []course.Assignment{
		{ID: 1, Course: "Software Engineering", Title: "Design review report", DueAt: time.Date(2026, 5, 20, 23, 59, 0, 0, time.Local), Done: false},
		{ID: 2, Course: "Database Systems", Title: "SQL practice", DueAt: time.Date(2026, 5, 18, 23, 59, 0, 0, time.Local), Done: true},
		{ID: 3, Course: "Computer Networks", Title: "Packet tracing lab", DueAt: time.Date(2026, 5, 15, 23, 59, 0, 0, time.Local), Done: false},
	}

	query := "software"
	results := course.SearchAssignments(assignments, query)
	if len(results) == 0 {
		fmt.Println(course.EmptySearchMessage(query))
		return
	}

	for _, item := range results {
		fmt.Printf("%s - %s\n", item.Course, item.Title)
	}
}
