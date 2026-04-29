package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
)

const (
	defaultPostCancelWait = 2 * time.Second
)

type RuntimeGateway interface {
	RunSync(ctx context.Context, request RuntimeTaskRequest) (RuntimeTaskExecution, error)
}

type RuntimeTaskRequest struct {
	SessionID          corepkg.SessionID
	TraceID            corepkg.TraceID
	Name               string
	Kind               string
	ParentTaskID       corepkg.TaskID
	Timeout            time.Duration
	Metadata           map[string]string
	Execute            func(context.Context) ([]byte, error)
	OnTaskStateChanged func(runtimepkg.Task)
}

type RuntimeTaskExecution struct {
	TaskID         corepkg.TaskID
	Result         runtimepkg.TaskResult
	ExecutionError error
}

type defaultRuntimeGateway struct {
	taskManager      runtimepkg.TaskManager
	postCancelWait   time.Duration
	tokenTimeFactory func() time.Time
}

func newDefaultRuntimeGateway(taskManager runtimepkg.TaskManager) RuntimeGateway {
	return &defaultRuntimeGateway{
		taskManager:      taskManager,
		postCancelWait:   defaultPostCancelWait,
		tokenTimeFactory: time.Now,
	}
}

func (g *defaultRuntimeGateway) RunSync(ctx context.Context, request RuntimeTaskRequest) (RuntimeTaskExecution, error) {
	if g == nil || g.taskManager == nil {
		return RuntimeTaskExecution{}, fmt.Errorf("runtime task manager is unavailable")
	}
	if request.Execute == nil {
		return RuntimeTaskExecution{}, fmt.Errorf("runtime task executor is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var (
		executionErr error
		execErrMu    sync.Mutex
	)

	executor := func(execCtx context.Context, task runtimepkg.Task) ([]byte, error) {
		if request.OnTaskStateChanged != nil {
			request.OnTaskStateChanged(task)
		}
		output, err := request.Execute(execCtx)
		if err != nil {
			execErrMu.Lock()
			executionErr = err
			execErrMu.Unlock()
		}
		return output, err
	}

	spec := runtimepkg.TaskSpec{
		SessionID:    request.SessionID,
		TraceID:      request.TraceID,
		Name:         request.Name,
		Kind:         request.Kind,
		ParentTaskID: request.ParentTaskID,
		Timeout:      request.Timeout,
		Metadata:     copyTaskMetadata(request.Metadata),
	}

	registry, ok := g.taskManager.(runtimepkg.TaskExecutionRegistry)
	if !ok {
		return RuntimeTaskExecution{}, fmt.Errorf("runtime task manager must implement TaskExecutionRegistry")
	}
	token := g.buildExecutionToken(spec)
	if spec.Metadata == nil {
		spec.Metadata = make(map[string]string, 1)
	}
	spec.Metadata[runtimepkg.TaskExecutionTokenMetadataKey] = token
	registry.RegisterExecution(token, executor)
	defer registry.UnregisterExecution(token)

	taskID, submitErr := g.taskManager.Submit(ctx, spec)
	if submitErr != nil {
		return RuntimeTaskExecution{}, submitErr
	}

	if request.OnTaskStateChanged != nil {
		if snapshot, err := g.taskManager.Get(context.Background(), taskID); err == nil {
			request.OnTaskStateChanged(snapshot)
		}
	}

	result, waitErr := g.taskManager.Wait(ctx, taskID)
	if waitErr == nil {
		execErrMu.Lock()
		executionErrCopy := executionErr
		execErrMu.Unlock()
		if request.OnTaskStateChanged != nil {
			if snapshot, err := g.taskManager.Get(context.Background(), taskID); err == nil {
				request.OnTaskStateChanged(snapshot)
			}
		}
		return RuntimeTaskExecution{
			TaskID:         taskID,
			Result:         result,
			ExecutionError: executionErrCopy,
		}, nil
	}

	execution := RuntimeTaskExecution{TaskID: taskID}
	if !errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, context.DeadlineExceeded) {
		return execution, waitErr
	}
	if ctx.Err() == nil {
		return execution, waitErr
	}

	_ = g.taskManager.Cancel(context.Background(), taskID, "parent_context_cancelled")

	settleWait := g.postCancelWait
	if settleWait <= 0 {
		settleWait = defaultPostCancelWait
	}
	settleCtx, settleCancel := context.WithTimeout(context.Background(), settleWait)
	defer settleCancel()

	settled, settleErr := g.taskManager.Wait(settleCtx, taskID)
	if settleErr != nil {
		return execution, waitErr
	}

	execErrMu.Lock()
	executionErrCopy := executionErr
	execErrMu.Unlock()
	execution.Result = settled
	execution.ExecutionError = executionErrCopy

	if request.OnTaskStateChanged != nil {
		if snapshot, err := g.taskManager.Get(context.Background(), taskID); err == nil {
			request.OnTaskStateChanged(snapshot)
		}
	}

	return execution, waitErr
}

func (g *defaultRuntimeGateway) buildExecutionToken(spec runtimepkg.TaskSpec) string {
	nowFn := g.tokenTimeFactory
	if nowFn == nil {
		nowFn = time.Now
	}
	parts := []string{
		strings.TrimSpace(string(spec.SessionID)),
		strings.TrimSpace(string(spec.TraceID)),
		strings.TrimSpace(spec.Name),
	}
	key := strings.Join(parts, ":")
	if parts[0] == "" && parts[1] == "" && parts[2] == "" {
		key = "task"
	}
	return fmt.Sprintf("%s:%d", key, nowFn().UTC().UnixNano())
}

type runtimeTaskResultError struct {
	status    corepkg.TaskStatus
	errorCode string
}

func (e runtimeTaskResultError) Error() string {
	switch e.status {
	case corepkg.TaskKilled:
		if e.errorCode != "" {
			return fmt.Sprintf("runtime task cancelled (%s)", e.errorCode)
		}
		return "runtime task cancelled"
	case corepkg.TaskFailed:
		if e.errorCode != "" {
			return fmt.Sprintf("runtime task failed (%s)", e.errorCode)
		}
		return "runtime task failed"
	default:
		if e.errorCode != "" {
			return fmt.Sprintf("runtime task ended with status %s (%s)", e.status, e.errorCode)
		}
		return fmt.Sprintf("runtime task ended with status %s", e.status)
	}
}

func copyTaskMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	copied := make(map[string]string, len(src))
	for key, value := range src {
		copied[key] = value
	}
	return copied
}
