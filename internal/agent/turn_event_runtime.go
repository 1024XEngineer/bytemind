package agent

import "context"

type turnEventSink interface {
	Emit(TurnEvent) error
}

type turnEventSinkContextKey struct{}

func withTurnEventSink(ctx context.Context, sink turnEventSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, turnEventSinkContextKey{}, sink)
}

func emitTurnEvent(ctx context.Context, event TurnEvent) {
	if ctx == nil {
		return
	}
	sink, ok := ctx.Value(turnEventSinkContextKey{}).(turnEventSink)
	if !ok || sink == nil {
		return
	}
	_ = sink.Emit(event)
}
