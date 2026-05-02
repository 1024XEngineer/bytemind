package notify

import "context"

type noopNotifier struct{}

func NewNoopNotifier() Notifier {
	return noopNotifier{}
}

func (noopNotifier) Notify(Message) {}

func (noopNotifier) Close(context.Context) error {
	return nil
}
