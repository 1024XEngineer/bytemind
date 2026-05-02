package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultQueueSize   = 32
	defaultSendTimeout = 2 * time.Second
)

type platformSender interface {
	Send(ctx context.Context, title, body string) error
}

type desktopNotifier struct {
	sender      platformSender
	queue       chan Message
	done        chan struct{}
	sendTimeout time.Duration
	cooldown    time.Duration
	now         func() time.Time

	mu     sync.Mutex
	closed bool
	recent map[string]time.Time
	drop   int64

	closeOnce sync.Once
}

func NewDesktopNotifier(cfg DesktopConfig) Notifier {
	if !cfg.Enabled {
		return NewNoopNotifier()
	}
	sender, ok := newPlatformSender()
	if !ok || sender == nil {
		return NewNoopNotifier()
	}
	return newDesktopNotifierWithSender(cfg, sender, time.Now)
}

func newDesktopNotifierWithSender(cfg DesktopConfig, sender platformSender, now func() time.Time) *desktopNotifier {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	sendTimeout := time.Duration(cfg.SendTimeoutMs) * time.Millisecond
	if sendTimeout <= 0 {
		sendTimeout = defaultSendTimeout
	}
	cooldownSeconds := cfg.CooldownSeconds
	if cooldownSeconds < 0 {
		cooldownSeconds = 0
	}
	notifier := &desktopNotifier{
		sender:      sender,
		queue:       make(chan Message, queueSize),
		done:        make(chan struct{}),
		sendTimeout: sendTimeout,
		cooldown:    time.Duration(cooldownSeconds) * time.Second,
		now:         now,
		recent:      make(map[string]time.Time, queueSize),
	}
	go notifier.run()
	return notifier
}

func (n *desktopNotifier) Notify(msg Message) {
	if n == nil {
		return
	}
	msg = normalizeMessage(msg)
	now := n.now()

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return
	}
	if n.cooldown > 0 && msg.Key != "" {
		if last, ok := n.recent[msg.Key]; ok && now.Sub(last) < n.cooldown {
			return
		}
	}
	n.enqueueLocked(msg)
	if msg.Key != "" {
		n.recent[msg.Key] = now
	}
}

func (n *desktopNotifier) Close(ctx context.Context) error {
	if n == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	n.closeOnce.Do(func() {
		n.mu.Lock()
		if !n.closed {
			n.closed = true
			close(n.queue)
		}
		n.mu.Unlock()
	})
	select {
	case <-n.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *desktopNotifier) droppedCount() int64 {
	if n == nil {
		return 0
	}
	return atomic.LoadInt64(&n.drop)
}

func (n *desktopNotifier) enqueueLocked(msg Message) {
	select {
	case n.queue <- msg:
		return
	default:
	}
	select {
	case <-n.queue:
		atomic.AddInt64(&n.drop, 1)
	default:
	}
	select {
	case n.queue <- msg:
	default:
		atomic.AddInt64(&n.drop, 1)
	}
}

func (n *desktopNotifier) run() {
	defer close(n.done)
	for msg := range n.queue {
		if n.sender == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), n.sendTimeout)
		_ = n.sender.Send(ctx, msg.Title, msg.Body)
		cancel()
	}
}
