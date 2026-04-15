package agent

import (
	"io"
	"strings"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

func (r *Runner) finalizeTurnWithoutTools(runMode planpkg.AgentMode, sess *session.Session, reply llm.Message, out io.Writer, streamedText bool) (string, error) {
	answer := strings.TrimSpace(reply.Content)
	if answer == "" {
		reply.Content = emptyReplyFallback
		answer = emptyReplyFallback
	}

	policyAnswer := planpkg.FinalizeAssistantAnswer(runMode, sess.Plan, answer)
	if policyAnswer != answer {
		answer = policyAnswer
		reply = llm.NewAssistantTextMessage(answer)
	}

	if err := r.persistAssistantReply(sess, reply); err != nil {
		return "", err
	}

	answer = strings.TrimSpace(reply.Content)
	if out != nil && !streamedText {
		_, _ = io.WriteString(out, "\n")
		_, _ = io.WriteString(out, answer+"\n")
	}
	return answer, nil
}

func (r *Runner) persistAssistantReply(sess *session.Session, reply llm.Message) error {
	if err := llm.ValidateMessage(reply); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, reply)
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return err
		}
	}
	r.emit(Event{
		Type:      EventAssistantMessage,
		SessionID: corepkg.SessionID(sess.ID),
		Content:   reply.Content,
	})
	r.emit(Event{
		Type:      EventRunFinished,
		SessionID: corepkg.SessionID(sess.ID),
		Content:   reply.Content,
	})
	return nil
}
