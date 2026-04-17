package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestInMemorySubAgentCoordinatorSpawnAndWaitSync(t *testing.T) {
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(_ context.Context, task Task) ([]byte, error) {
			return []byte("child:" + task.Spec.Name), nil
		}),
	)
	quota := NewInMemoryQuotaManager(1, nil)
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	taskID, err := coordinator.Spawn(context.Background(), TaskSpec{
		SessionID: "sess-1",
		Name:      "demo-child",
		Metadata: map[string]string{
			"quota_key": "subagent",
		},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	result, err := coordinator.Wait(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if string(result.Output) != "child:demo-child" {
		t.Fatalf("expected output %q, got %q", "child:demo-child", string(result.Output))
	}
}

func TestInMemorySubAgentCoordinatorQuotaExceededDoesNotSubmitTask(t *testing.T) {
	blocker := make(chan struct{})
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
			select {
			case <-blocker:
				return []byte("released"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	quota := NewInMemoryQuotaManager(1, map[string]int{"shared": 1})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	firstID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "first",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}

	// Ensure the first task has started and holds quota.
	waitUntilTaskStatus(t, manager, firstID, corepkg.TaskRunning, 2*time.Second)

	_, err = coordinator.Spawn(context.Background(), TaskSpec{
		Name: "second",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err == nil {
		t.Fatal("expected quota exceeded error for second spawn")
	}
	if !hasErrorCode(err, ErrorCodeQuotaExceeded) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeQuotaExceeded, errorCode(err))
	}

	manager.mu.RLock()
	taskCount := len(manager.tasks)
	manager.mu.RUnlock()
	if taskCount != 1 {
		t.Fatalf("expected only one submitted task, got %d", taskCount)
	}

	close(blocker)
	_, err = coordinator.Wait(context.Background(), firstID)
	if err != nil {
		t.Fatalf("Wait first task failed: %v", err)
	}
}

func TestInMemoryTaskManagerParentCancelPropagatesToChild(t *testing.T) {
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, task Task) ([]byte, error) {
			_ = task
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	)

	parentID, err := manager.Submit(context.Background(), TaskSpec{
		Name: "parent",
	})
	if err != nil {
		t.Fatalf("Submit parent failed: %v", err)
	}
	childID, err := manager.Submit(context.Background(), TaskSpec{
		Name:         "child",
		ParentTaskID: parentID,
	})
	if err != nil {
		t.Fatalf("Submit child failed: %v", err)
	}

	waitUntilTaskStatus(t, manager, parentID, corepkg.TaskRunning, 2*time.Second)
	waitUntilTaskStatus(t, manager, childID, corepkg.TaskRunning, 2*time.Second)

	if err := manager.Cancel(context.Background(), parentID, "cancel parent"); err != nil {
		t.Fatalf("Cancel parent failed: %v", err)
	}

	parentResult, err := manager.Wait(context.Background(), parentID)
	if err != nil {
		t.Fatalf("Wait parent failed: %v", err)
	}
	childResult, err := manager.Wait(context.Background(), childID)
	if err != nil {
		t.Fatalf("Wait child failed: %v", err)
	}

	if parentResult.Status != corepkg.TaskKilled {
		t.Fatalf("expected parent killed, got %s", parentResult.Status)
	}
	if childResult.Status != corepkg.TaskKilled {
		t.Fatalf("expected child killed, got %s", childResult.Status)
	}
	if childResult.ErrorCode != ErrorCodeTaskCancelled {
		t.Fatalf("expected child cancel error code %q, got %q", ErrorCodeTaskCancelled, childResult.ErrorCode)
	}
}

func TestInMemorySubAgentCoordinatorQuotaContentionConverges(t *testing.T) {
	release := make(chan struct{})
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
			select {
			case <-release:
				return []byte("released"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	quota := NewInMemoryQuotaManager(2, map[string]int{"shared": 2})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	const totalSpawns = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	type spawnResult struct {
		id  corepkg.TaskID
		err error
	}
	results := make(chan spawnResult, totalSpawns)

	for i := 0; i < totalSpawns; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			id, err := coordinator.Spawn(context.Background(), TaskSpec{
				Name: "contended-child",
				Metadata: map[string]string{
					"quota_key": "shared",
				},
			})
			_ = index
			results <- spawnResult{id: id, err: err}
		}(i)
	}

	close(start)
	wg.Wait()
	close(results)

	successIDs := make([]corepkg.TaskID, 0, 2)
	quotaExceeded := 0
	for result := range results {
		if result.err == nil {
			successIDs = append(successIDs, result.id)
			continue
		}
		if !hasErrorCode(result.err, ErrorCodeQuotaExceeded) {
			t.Fatalf("expected quota exceeded error, got %v", result.err)
		}
		quotaExceeded++
	}

	if len(successIDs) != 2 {
		t.Fatalf("expected exactly 2 successful spawns under quota=2, got %d", len(successIDs))
	}
	if quotaExceeded != totalSpawns-len(successIDs) {
		t.Fatalf("unexpected quota-exceeded count: got %d want %d", quotaExceeded, totalSpawns-len(successIDs))
	}

	for _, id := range successIDs {
		waitUntilTaskStatus(t, manager, id, corepkg.TaskRunning, 2*time.Second)
	}
	close(release)

	for _, id := range successIDs {
		result, err := coordinator.Wait(context.Background(), id)
		if err != nil {
			t.Fatalf("wait task %q failed: %v", id, err)
		}
		if result.Status != corepkg.TaskCompleted {
			t.Fatalf("expected completed status for %q, got %s", id, result.Status)
		}
	}

	// Quota should be released after Wait and allow new spawn.
	nextID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "post-release-child",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("expected spawn to recover after release, got %v", err)
	}
	nextResult, err := coordinator.Wait(context.Background(), nextID)
	if err != nil {
		t.Fatalf("expected post-release wait success, got %v", err)
	}
	if nextResult.Status != corepkg.TaskCompleted {
		t.Fatalf("expected post-release completed status, got %s", nextResult.Status)
	}
}

func TestInMemorySubAgentCoordinatorWaitCancellationDoesNotReleaseQuotaEarly(t *testing.T) {
	release := make(chan struct{})
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
			select {
			case <-release:
				return []byte("released"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	quota := NewInMemoryQuotaManager(1, map[string]int{"shared": 1})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	taskID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "first",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("spawn first failed: %v", err)
	}
	waitUntilTaskStatus(t, manager, taskID, corepkg.TaskRunning, 2*time.Second)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer waitCancel()
	_, err = coordinator.Wait(waitCtx, taskID)
	if err == nil {
		t.Fatal("expected wait timeout for running task")
	}

	_, err = coordinator.Spawn(context.Background(), TaskSpec{
		Name: "second-before-terminal",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err == nil {
		t.Fatal("expected quota to remain occupied until first task reaches terminal state")
	}
	if !hasErrorCode(err, ErrorCodeQuotaExceeded) {
		t.Fatalf("expected quota exceeded error, got %v", err)
	}

	close(release)
	if _, err := manager.Wait(context.Background(), taskID); err != nil {
		t.Fatalf("wait first task completion failed: %v", err)
	}

	waitUntilCondition(t, 2*time.Second, func() bool {
		_, spawnErr := coordinator.Spawn(context.Background(), TaskSpec{
			Name: "post-terminal",
			Metadata: map[string]string{
				"quota_key": "shared",
			},
		})
		if spawnErr == nil {
			return true
		}
		return false
	})
}

func TestInMemorySubAgentCoordinatorReleasesQuotaWithoutCallerWait(t *testing.T) {
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(_ context.Context, _ Task) ([]byte, error) {
			time.Sleep(20 * time.Millisecond)
			return []byte("done"), nil
		}),
	)
	quota := NewInMemoryQuotaManager(1, map[string]int{"shared": 1})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	taskID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "first-no-wait",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("spawn first failed: %v", err)
	}

	waitUntilTaskStatus(t, manager, taskID, corepkg.TaskCompleted, 2*time.Second)

	waitUntilCondition(t, 2*time.Second, func() bool {
		nextID, spawnErr := coordinator.Spawn(context.Background(), TaskSpec{
			Name: "second-after-auto-release",
			Metadata: map[string]string{
				"quota_key": "shared",
			},
		})
		if spawnErr != nil {
			return false
		}
		if _, waitErr := coordinator.Wait(context.Background(), nextID); waitErr != nil {
			t.Fatalf("wait second task failed: %v", waitErr)
		}
		return true
	})
}

func waitUntilTaskStatus(t *testing.T, manager *InMemoryTaskManager, id corepkg.TaskID, expected corepkg.TaskStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		task, err := manager.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get task %q failed: %v", id, err)
		}
		if task.Status == expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %q did not reach status %s (current=%s)", id, expected, task.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitUntilCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("condition was not satisfied before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
