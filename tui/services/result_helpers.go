package services

import (
	"strings"

	"bytemind/tui/api"
	tuiruntime "bytemind/tui/runtime"
)

func failResult[T any](service string, err error) api.Result[T] {
	if err == nil {
		return api.Fail[T]("unknown service error")
	}
	if tuiruntime.IsUnavailableError(err) {
		return api.Unavailable[T](service)
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "unknown service error"
	}
	return api.FailCode[T](api.ErrorCodeInternal, message)
}
