package api

type ErrorCode string

const (
	ErrorCodeUnknown     ErrorCode = "unknown"
	ErrorCodeUnavailable ErrorCode = "unavailable"
	ErrorCodeInvalid     ErrorCode = "invalid"
	ErrorCodeInternal    ErrorCode = "internal"
)

type Result[T any] struct {
	Success bool
	Data    T
	Error   string
	Code    ErrorCode
}

func Ok[T any](data T) Result[T] {
	return Result[T]{
		Success: true,
		Data:    data,
		Code:    ErrorCodeUnknown,
	}
}

func Fail[T any](msg string) Result[T] {
	return Result[T]{
		Success: false,
		Error:   msg,
		Code:    ErrorCodeUnknown,
	}
}

func FailCode[T any](code ErrorCode, msg string) Result[T] {
	return Result[T]{
		Success: false,
		Error:   msg,
		Code:    code,
	}
}

func Invalid[T any](msg string) Result[T] {
	return FailCode[T](ErrorCodeInvalid, msg)
}

func Unavailable[T any](service string) Result[T] {
	msg := "service unavailable"
	if service != "" {
		msg = service + " unavailable"
	}
	return FailCode[T](ErrorCodeUnavailable, msg)
}

func (r Result[T]) IsUnavailable() bool {
	return !r.Success && r.Code == ErrorCodeUnavailable
}
