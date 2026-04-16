package session

import (
	"fmt"
	"testing"

	"bytemind/internal/llm"
)

type summaryTitleStringer struct {
	value string
}

func (s summaryTitleStringer) String() string {
	return s.value
}

func TestSessionTitlePrefersFieldThenMetaFallback(t *testing.T) {
	t.Run("nil session", func(t *testing.T) {
		if got := sessionTitle(nil); got != "" {
			t.Fatalf("expected empty title for nil session, got %q", got)
		}
	})

	t.Run("session title field", func(t *testing.T) {
		sess := &Session{Title: "  explicit title  "}
		if got := sessionTitle(sess); got != "explicit title" {
			t.Fatalf("expected trimmed title field, got %q", got)
		}
	})

	t.Run("meta nil or missing", func(t *testing.T) {
		sess := &Session{}
		if got := sessionTitle(sess); got != "" {
			t.Fatalf("expected empty title when meta missing, got %q", got)
		}
		sess.Conversation.Meta = ConversationMeta{}
		if got := sessionTitle(sess); got != "" {
			t.Fatalf("expected empty title when meta key missing, got %q", got)
		}
		sess.Conversation.Meta["title"] = nil
		if got := sessionTitle(sess); got != "" {
			t.Fatalf("expected empty title when meta title is nil, got %q", got)
		}
	})

	t.Run("meta title types", func(t *testing.T) {
		sess := &Session{Conversation: Conversation{Meta: ConversationMeta{"title": "  from string  "}}}
		if got := sessionTitle(sess); got != "from string" {
			t.Fatalf("expected string meta title, got %q", got)
		}

		sess.Conversation.Meta["title"] = summaryTitleStringer{value: "  from stringer  "}
		if got := sessionTitle(sess); got != "from stringer" {
			t.Fatalf("expected fmt.Stringer title, got %q", got)
		}

		sess.Conversation.Meta["title"] = 42
		if got := sessionTitle(sess); got != fmt.Sprint(42) {
			t.Fatalf("expected default fmt.Sprint fallback, got %q", got)
		}
	})
}

func TestSessionTimelinePrefersConversationTimeline(t *testing.T) {
	if got := sessionTimeline(nil); got != nil {
		t.Fatalf("expected nil timeline for nil session, got %#v", got)
	}

	msgFromMessages := llm.NewUserTextMessage("from messages")
	msgFromTimeline := llm.NewUserTextMessage("from timeline")
	sess := &Session{
		Messages: []llm.Message{msgFromMessages},
	}
	if got := sessionTimeline(sess); len(got) != 1 || got[0].Content != msgFromMessages.Content {
		t.Fatalf("expected fallback to Messages when timeline is empty, got %#v", got)
	}

	sess.Conversation.Timeline = []llm.Message{msgFromTimeline}
	if got := sessionTimeline(sess); len(got) != 1 || got[0].Content != msgFromTimeline.Content {
		t.Fatalf("expected Conversation.Timeline to take priority, got %#v", got)
	}
}

func TestSummarizeMessageHandlesTinyLimitWithoutEllipsis(t *testing.T) {
	if got := summarizeMessage("  hello world  ", 3); got != "hel" {
		t.Fatalf("expected tiny limit to truncate without ellipsis, got %q", got)
	}
}
