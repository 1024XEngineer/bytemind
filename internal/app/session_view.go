package app

import (
	"time"

	"bytemind/internal/session"
)

type SessionSnapshotView struct {
	ID        string
	Workspace string
	Updated   string
}

type SessionSummaryView struct {
	Marker       string
	ID           string
	Updated      string
	MessageCount int
	Preview      string
	Workspace    string
}

func BuildSessionSnapshotView(sess *session.Session, location *time.Location) SessionSnapshotView {
	if location == nil {
		location = time.Local
	}
	if sess == nil {
		return SessionSnapshotView{}
	}
	return SessionSnapshotView{
		ID:        sess.ID,
		Workspace: sess.Workspace,
		Updated:   sess.UpdatedAt.In(location).Format(time.DateTime),
	}
}

func BuildSessionSummaryViews(summaries []session.Summary, currentID string, location *time.Location) []SessionSummaryView {
	if location == nil {
		location = time.Local
	}
	if len(summaries) == 0 {
		return nil
	}
	views := make([]SessionSummaryView, 0, len(summaries))
	for _, item := range summaries {
		marker := " "
		if item.ID == currentID {
			marker = "*"
		}
		preview := item.LastUserMessage
		if preview == "" {
			preview = "(no user prompt yet)"
		}
		views = append(views, SessionSummaryView{
			Marker:       marker,
			ID:           item.ID,
			Updated:      item.UpdatedAt.In(location).Format("2006-01-02 15:04"),
			MessageCount: item.MessageCount,
			Preview:      preview,
			Workspace:    item.Workspace,
		})
	}
	return views
}
