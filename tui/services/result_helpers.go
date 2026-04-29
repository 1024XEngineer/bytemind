package services

import (
	"strings"

	"github.com/1024XEngineer/bytemind/tui/api"
	tuiruntime "github.com/1024XEngineer/bytemind/tui/runtime"
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
