package agent

import (
	"errors"
	"fmt"

	"bytemind/internal/llm"
)

var errPromptTooLong = errors.New("prompt too long")

func newPromptTooLongError(promptTokens, tokenQuota int, criticalRatio float64) error {
	return fmt.Errorf(
		"%w (%d estimated tokens) exceeded critical context budget %.0f%% of quota %d. Try a shorter prompt or split it",
		errPromptTooLong,
		promptTokens,
		criticalRatio*100,
		tokenQuota,
	)
}

func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	if isLocalPromptTooLongError(err) {
		return true
	}

	var providerErr *llm.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Code == llm.ErrorCodeContextTooLong {
			return true
		}
		if llm.IsContextTooLongMessage(providerErr.Message) {
			return true
		}
	}

	return llm.IsContextTooLongMessage(err.Error())
}

func isLocalPromptTooLongError(err error) bool {
	return errors.Is(err, errPromptTooLong)
}
