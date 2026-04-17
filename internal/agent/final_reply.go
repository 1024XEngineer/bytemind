package agent

import (
	"fmt"
	"io"
	"strings"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

func (e *defaultEngine) finalizeTurnWithoutTools(runMode planpkg.AgentMode, sess *session.Session, reply llm.Message, out io.Writer, streamedText bool) (string, error) {
	if e == nil || e.runner == nil {
		return "", fmt.Errorf("agent engine is unavailable")
	}

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

	if err := e.persistAssistantReply(sess, reply); err != nil {
		return "", err
	}

	answer = strings.TrimSpace(reply.Content)
	if out != nil && !streamedText {
		_, _ = io.WriteString(out, "\n")
		_, _ = io.WriteString(out, answer+"\n")
	}
	return answer, nil
}

func (e *defaultEngine) persistAssistantReply(sess *session.Session, reply llm.Message) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	if err := llm.ValidateMessage(reply); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, reply)
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	runner.emit(Event{
		Type:      EventAssistantMessage,
		SessionID: corepkg.SessionID(sess.ID),
		Content:   reply.Content,
	})
	runner.emit(Event{
		Type:      EventRunFinished,
		SessionID: corepkg.SessionID(sess.ID),
		Content:   reply.Content,
	})
	return nil
}

func (r *Runner) finalizeTurnWithoutTools(runMode planpkg.AgentMode, sess *session.Session, reply llm.Message, out io.Writer, streamedText bool) (string, error) {
	engine := &defaultEngine{runner: r}
	return engine.finalizeTurnWithoutTools(runMode, sess, reply, out, streamedText)
}

func (r *Runner) persistAssistantReply(sess *session.Session, reply llm.Message) error {
	engine := &defaultEngine{runner: r}
	return engine.persistAssistantReply(sess, reply)
}
