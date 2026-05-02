package runtime

import (
	"context"
	"sync"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type InMemorySubAgentCoordinator struct {
	taskManager  TaskManager
	quotaManager QuotaManager

	mu        sync.Mutex
	quotaKeys map[corepkg.TaskID]string
}

func NewInMemorySubAgentCoordinator(taskManager TaskManager, quotaManager QuotaManager) *InMemorySubAgentCoordinator {
	return &InMemorySubAgentCoordinator{
		taskManager:  taskManager,
		quotaManager: quotaManager,
		quotaKeys:    make(map[corepkg.TaskID]string),
	}
}

func (c *InMemorySubAgentCoordinator) Spawn(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error) {
	if c == nil || c.taskManager == nil {
		return "", ErrTaskNotImplemented
	}
	if ctx == nil {
		ctx = context.Background()
	}

	quotaKey := deriveQuotaKey(spec)
	if c.quotaManager != nil {
		if err := c.quotaManager.Acquire(ctx, quotaKey); err != nil {
			return "", err
		}
	}

	taskID, err := c.taskManager.Submit(ctx, spec)
	if err != nil {
		if c.quotaManager != nil {
			c.quotaManager.Release(quotaKey)
		}
		return "", err
	}

	if c.quotaManager != nil {
		c.mu.Lock()
		c.quotaKeys[taskID] = quotaKey
		c.mu.Unlock()
		go c.waitTerminalAndRelease(taskID)
	}

	return taskID, nil
}

func (c *InMemorySubAgentCoordinator) Wait(ctx context.Context, id corepkg.TaskID) (TaskResult, error) {
	if c == nil || c.taskManager == nil {
		return TaskResult{}, ErrTaskNotImplemented
	}
	result, err := c.taskManager.Wait(ctx, id)
	if err == nil {
		if IsTerminalTaskStatus(result.Status) {
			c.releaseQuota(id)
		}
		return result, nil
	}

	if c.isTaskTerminal(id) {
		c.releaseQuota(id)
	}
	return result, err
}

func (c *InMemorySubAgentCoordinator) waitTerminalAndRelease(id corepkg.TaskID) {
	if c == nil || c.taskManager == nil || c.quotaManager == nil {
		return
	}
	_, err := c.taskManager.Wait(context.Background(), id)
	if err != nil && !c.isTaskTerminal(id) {
		return
	}
	c.releaseQuota(id)
}

func (c *InMemorySubAgentCoordinator) isTaskTerminal(id corepkg.TaskID) bool {
	if c == nil || c.taskManager == nil {
		return false
	}
	task, err := c.taskManager.Get(context.Background(), id)
	if err != nil {
		return false
	}
	return IsTerminalTaskStatus(task.Status)
}

func (c *InMemorySubAgentCoordinator) releaseQuota(id corepkg.TaskID) {
	if c == nil || c.quotaManager == nil {
		return
	}
	c.mu.Lock()
	quotaKey, ok := c.quotaKeys[id]
	if ok {
		delete(c.quotaKeys, id)
	}
	c.mu.Unlock()
	if ok {
		c.quotaManager.Release(quotaKey)
	}
}

func deriveQuotaKey(spec TaskSpec) string {
	if spec.Metadata != nil {
		if key := spec.Metadata["quota_key"]; key != "" {
			return key
		}
	}
	if spec.SessionID != "" {
		return "session:" + string(spec.SessionID)
	}
	if spec.Kind != "" {
		return "kind:" + spec.Kind
	}
	return "global"
}
