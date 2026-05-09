package rollback

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

const maxSnapshotBytes int64 = 5 * 1024 * 1024

type OpType string

const (
	OpTypeAdd    OpType = "add"
	OpTypeUpdate OpType = "update"
	OpTypeDelete OpType = "delete"
	OpTypeMove   OpType = "move"
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusCommitted      Status = "committed"
	StatusRolledBack     Status = "rolled_back"
	StatusRollbackFailed Status = "rollback_failed"
	StatusAborted        Status = "aborted"
)

type BeginOptions struct {
	Workspace string
	SessionID string
	TaskID    string
	TraceID   string
	ToolName  string
	Actor     string
}

type FileTarget struct {
	Path       string
	AbsPath    string
	NewPath    string
	NewAbsPath string
	OpType     OpType
}

type Operation struct {
	OperationID      string       `json:"operation_id"`
	SessionID        string       `json:"session_id,omitempty"`
	TaskID           string       `json:"task_id,omitempty"`
	TraceID          string       `json:"trace_id,omitempty"`
	Workspace        string       `json:"workspace"`
	ToolName         string       `json:"tool_name"`
	Actor            string       `json:"actor,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Status           Status       `json:"status"`
	AffectedFiles    []FileChange `json:"affected_files"`
	RollbackAttempts int          `json:"rollback_attempts"`
	LastError        string       `json:"last_error,omitempty"`
}

type FileChange struct {
	Path             string `json:"path"`
	AbsPath          string `json:"abs_path,omitempty"`
	NewPath          string `json:"new_path,omitempty"`
	NewAbsPath       string `json:"new_abs_path,omitempty"`
	OpType           OpType `json:"op_type"`
	FileAbsentBefore bool   `json:"file_absent_before,omitempty"`
	FileAbsentAfter  bool   `json:"file_absent_after,omitempty"`
	BeforeHash       string `json:"before_hash,omitempty"`
	AfterHash        string `json:"after_hash,omitempty"`
	BeforeSnapshot   string `json:"before_snapshot,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"`
}

type Store struct {
	root       string
	entriesDir string
	blobsDir   string
}

func NewDefaultStore() (*Store, error) {
	home, err := configpkg.ResolveHomeDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(home, "rollback"))
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("rollback root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	store := &Store{
		root:       root,
		entriesDir: filepath.Join(root, "entries"),
		blobsDir:   filepath.Join(root, "blobs"),
	}
	if err := os.MkdirAll(store.entriesDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.blobsDir, 0o755); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Begin(ctx context.Context, opts BeginOptions, targets []FileTarget) (*Operation, error) {
	if s == nil {
		return nil, errors.New("rollback store is unavailable")
	}
	if len(targets) == 0 {
		return nil, errors.New("rollback operation requires at least one file")
	}
	workspace, err := filepath.Abs(strings.TrimSpace(opts.Workspace))
	if err != nil {
		return nil, err
	}
	workspace = filepath.Clean(workspace)
	now := time.Now().UTC()
	op := &Operation{
		OperationID:   newOperationID(now),
		SessionID:     strings.TrimSpace(opts.SessionID),
		TaskID:        strings.TrimSpace(opts.TaskID),
		TraceID:       strings.TrimSpace(opts.TraceID),
		Workspace:     workspace,
		ToolName:      strings.TrimSpace(opts.ToolName),
		Actor:         strings.TrimSpace(opts.Actor),
		CreatedAt:     now,
		UpdatedAt:     now,
		Status:        StatusPending,
		AffectedFiles: make([]FileChange, 0, len(targets)),
	}
	if op.Actor == "" {
		op.Actor = "agent"
	}

	for i, target := range targets {
		change, err := s.captureBeforeState(op, workspace, i, target)
		if err != nil {
			return nil, err
		}
		op.AffectedFiles = append(op.AffectedFiles, change)
	}

	if err := s.saveOperation(op); err != nil {
		return nil, err
	}
	return op, nil
}

func (s *Store) Commit(ctx context.Context, op *Operation) error {
	if s == nil || op == nil {
		return nil
	}
	for i := range op.AffectedFiles {
		change := &op.AffectedFiles[i]
		switch change.OpType {
		case OpTypeAdd, OpTypeUpdate:
			hash, absent, err := hashCurrentFile(resolveChangePath(*op, *change))
			if err != nil {
				return err
			}
			if absent {
				return fmt.Errorf("rollback commit failed: %s is absent after %s", change.Path, change.OpType)
			}
			change.AfterHash = hash
			change.FileAbsentAfter = false
		case OpTypeDelete:
			_, absent, err := hashCurrentFile(resolveChangePath(*op, *change))
			if err != nil {
				return err
			}
			if !absent {
				return fmt.Errorf("rollback commit failed: %s still exists after delete", change.Path)
			}
			change.AfterHash = ""
			change.FileAbsentAfter = true
		case OpTypeMove:
			oldPath := resolveChangePath(*op, *change)
			newPath := resolveNewChangePath(*op, *change)
			_, oldAbsent, err := hashCurrentFile(oldPath)
			if err != nil {
				return err
			}
			if !oldAbsent {
				return fmt.Errorf("rollback commit failed: %s still exists after move", change.Path)
			}
			hash, newAbsent, err := hashCurrentFile(newPath)
			if err != nil {
				return err
			}
			if newAbsent {
				return fmt.Errorf("rollback commit failed: %s is absent after move", change.NewPath)
			}
			change.AfterHash = hash
			change.FileAbsentAfter = true
		default:
			return fmt.Errorf("unsupported rollback op type %q", change.OpType)
		}
	}
	op.Status = StatusCommitted
	op.UpdatedAt = time.Now().UTC()
	op.LastError = ""
	if err := s.saveOperation(op); err != nil {
		return err
	}
	s.appendAudit(ctx, op, "rollback_operation_committed", "success", "")
	return nil
}

func (s *Store) Abort(ctx context.Context, op *Operation, reason string) error {
	if s == nil || op == nil {
		return nil
	}
	op.Status = StatusAborted
	op.UpdatedAt = time.Now().UTC()
	op.LastError = strings.TrimSpace(reason)
	if err := s.saveOperation(op); err != nil {
		return err
	}
	s.appendAudit(ctx, op, "rollback_operation_aborted", "aborted", reason)
	return nil
}

func (s *Store) AbortAndRestore(ctx context.Context, op *Operation, reason string, writableRoots ...string) error {
	if s == nil || op == nil {
		return nil
	}
	restoreErr := s.restoreBefore(op, writableRoots...)
	status := StatusAborted
	lastError := strings.TrimSpace(reason)
	result := "aborted"
	if restoreErr != nil {
		status = StatusRollbackFailed
		result = "restore_failed"
		if lastError != "" {
			lastError += "; "
		}
		lastError += "automatic restore failed: " + restoreErr.Error()
	}
	op.Status = status
	op.UpdatedAt = time.Now().UTC()
	op.LastError = lastError
	if err := s.saveOperation(op); err != nil && restoreErr == nil {
		return err
	}
	s.appendAudit(ctx, op, "rollback_operation_aborted", result, lastError)
	return restoreErr
}

func (s *Store) ListRecent(ctx context.Context, workspace string, limit int) ([]Operation, error) {
	if s == nil {
		return nil, errors.New("rollback store is unavailable")
	}
	if limit <= 0 {
		limit = 10
	}
	ops, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	filtered := make([]Operation, 0, len(ops))
	for _, op := range ops {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if op.Status != StatusCommitted {
			continue
		}
		if !sameWorkspace(op.Workspace, workspace) {
			continue
		}
		filtered = append(filtered, op)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *Store) RollbackLast(ctx context.Context, workspace string, writableRoots ...string) (*Operation, error) {
	ops, err := s.ListRecent(ctx, workspace, 1)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, errors.New("no committed rollback operation found for this workspace")
	}
	return s.Rollback(ctx, workspace, ops[0].OperationID, writableRoots...)
}

func (s *Store) Rollback(ctx context.Context, workspace, operationID string, writableRoots ...string) (*Operation, error) {
	if s == nil {
		return nil, errors.New("rollback store is unavailable")
	}
	op, err := s.findOperation(workspace, operationID)
	if err != nil {
		return nil, err
	}
	if op.Status != StatusCommitted {
		return nil, fmt.Errorf("rollback operation %s is %s, not committed", op.OperationID, op.Status)
	}
	if err := validateOperationPaths(op, workspace, writableRoots...); err != nil {
		return nil, err
	}
	op.RollbackAttempts++
	op.UpdatedAt = time.Now().UTC()

	if err := checkConflicts(op); err != nil {
		op.LastError = err.Error()
		_ = s.saveOperation(&op)
		s.appendAudit(ctx, &op, "rollback_operation_blocked", "conflict", err.Error())
		return nil, err
	}

	current, err := captureCurrentStates(op)
	if err != nil {
		op.LastError = err.Error()
		_ = s.saveOperation(&op)
		return nil, err
	}
	if err := s.applyRollback(&op); err != nil {
		restoreErr := restoreCurrentStates(current)
		op.Status = StatusRollbackFailed
		op.LastError = err.Error()
		if restoreErr != nil {
			op.LastError += "; failed to restore rollback attempt state: " + restoreErr.Error()
		}
		_ = s.saveOperation(&op)
		s.appendAudit(ctx, &op, "rollback_operation_failed", "failed", op.LastError)
		return nil, err
	}
	op.Status = StatusRolledBack
	op.UpdatedAt = time.Now().UTC()
	op.LastError = ""
	if err := s.saveOperation(&op); err != nil {
		return nil, err
	}
	s.appendAudit(ctx, &op, "rollback_operation_executed", "success", "")
	return &op, nil
}

func (s *Store) captureBeforeState(op *Operation, workspace string, index int, target FileTarget) (FileChange, error) {
	opType := target.OpType
	if opType == "" {
		opType = OpTypeUpdate
	}
	absPath, err := normalizeTargetPath(workspace, target.AbsPath, target.Path)
	if err != nil {
		return FileChange{}, err
	}
	path := strings.TrimSpace(target.Path)
	if path == "" {
		path = displayPath(workspace, absPath)
	}
	change := FileChange{
		Path:    filepath.ToSlash(path),
		AbsPath: absPath,
		OpType:  opType,
	}
	if opType == OpTypeMove {
		newAbs, err := normalizeTargetPath(workspace, target.NewAbsPath, target.NewPath)
		if err != nil {
			return FileChange{}, err
		}
		newPath := strings.TrimSpace(target.NewPath)
		if newPath == "" {
			newPath = displayPath(workspace, newAbs)
		}
		change.NewPath = filepath.ToSlash(newPath)
		change.NewAbsPath = newAbs
	}

	state, err := readSnapshotCandidate(absPath)
	if err != nil {
		return FileChange{}, err
	}
	change.FileAbsentBefore = !state.exists
	if !state.exists {
		return change, nil
	}
	change.BeforeHash = hashBytes(state.data)
	change.SizeBytes = int64(len(state.data))
	blobRel := filepath.ToSlash(filepath.Join(op.OperationID, fmt.Sprintf("%03d.blob", index)))
	blobAbs := filepath.Join(s.blobsDir, filepath.FromSlash(blobRel))
	if err := os.MkdirAll(filepath.Dir(blobAbs), 0o755); err != nil {
		return FileChange{}, err
	}
	if err := os.WriteFile(blobAbs, state.data, 0o644); err != nil {
		return FileChange{}, err
	}
	change.BeforeSnapshot = blobRel
	return change, nil
}

func (s *Store) restoreBefore(op *Operation, writableRoots ...string) error {
	if op == nil {
		return nil
	}
	if err := validateOperationPaths(*op, op.Workspace, writableRoots...); err != nil {
		return err
	}
	current, err := captureCurrentStates(*op)
	if err != nil {
		return err
	}
	if err := s.applyBeforeState(op); err != nil {
		if restoreErr := restoreCurrentStates(current); restoreErr != nil {
			return fmt.Errorf("%w; also failed to restore current state: %v", err, restoreErr)
		}
		return err
	}
	return nil
}

func (s *Store) applyRollback(op *Operation) error {
	return s.applyBeforeState(op)
}

func (s *Store) applyBeforeState(op *Operation) error {
	if op == nil {
		return nil
	}
	for _, change := range op.AffectedFiles {
		path := resolveChangePath(*op, change)
		switch change.OpType {
		case OpTypeAdd:
			if err := removeIfExists(path); err != nil {
				return err
			}
		case OpTypeUpdate, OpTypeDelete:
			if err := s.restoreSnapshot(path, change); err != nil {
				return err
			}
		case OpTypeMove:
			if err := removeIfExists(resolveNewChangePath(*op, change)); err != nil {
				return err
			}
			if err := s.restoreSnapshot(path, change); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported rollback op type %q", change.OpType)
		}
	}
	return nil
}

func (s *Store) restoreSnapshot(path string, change FileChange) error {
	if change.FileAbsentBefore {
		return removeIfExists(path)
	}
	if strings.TrimSpace(change.BeforeSnapshot) == "" {
		return fmt.Errorf("rollback snapshot missing for %s", change.Path)
	}
	data, err := os.ReadFile(filepath.Join(s.blobsDir, filepath.FromSlash(change.BeforeSnapshot)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) loadAll() ([]Operation, error) {
	entries, err := os.ReadDir(s.entriesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	ops := make([]Operation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		op, err := s.loadOperationFile(filepath.Join(s.entriesDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func (s *Store) findOperation(workspace, operationID string) (Operation, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return Operation{}, errors.New("rollback operation id is required")
	}
	ops, err := s.loadAll()
	if err != nil {
		return Operation{}, err
	}
	matches := make([]Operation, 0, 1)
	for _, op := range ops {
		if !sameWorkspace(op.Workspace, workspace) {
			continue
		}
		if op.OperationID == operationID || strings.HasPrefix(op.OperationID, operationID) {
			matches = append(matches, op)
		}
	}
	if len(matches) == 0 {
		return Operation{}, fmt.Errorf("rollback operation %s was not found for this workspace", operationID)
	}
	if len(matches) > 1 {
		return Operation{}, fmt.Errorf("rollback operation id %s is ambiguous", operationID)
	}
	return matches[0], nil
}

func (s *Store) loadOperationFile(path string) (Operation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Operation{}, err
	}
	var op Operation
	if err := json.Unmarshal(data, &op); err != nil {
		return Operation{}, fmt.Errorf("read rollback operation %s: %w", path, err)
	}
	return op, nil
}

func (s *Store) saveOperation(op *Operation) error {
	if s == nil || op == nil {
		return nil
	}
	if strings.TrimSpace(op.OperationID) == "" {
		return errors.New("rollback operation id is required")
	}
	op.UpdatedAt = op.UpdatedAt.UTC()
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	} else {
		op.CreatedAt = op.CreatedAt.UTC()
	}
	path := filepath.Join(s.entriesDir, op.OperationID+".json")
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) appendAudit(ctx context.Context, op *Operation, action, result, reason string) {
	if op == nil {
		return
	}
	audit, err := storagepkg.NewDefaultAuditStore()
	if err != nil {
		return
	}
	metadata := map[string]string{
		"operation_id": op.OperationID,
		"tool_name":    op.ToolName,
		"workspace":    op.Workspace,
		"file_count":   strconv.Itoa(len(op.AffectedFiles)),
	}
	if strings.TrimSpace(reason) != "" {
		metadata["reason"] = strings.TrimSpace(reason)
	}
	_ = audit.Append(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(op.SessionID),
		TaskID:    corepkg.TaskID(op.TaskID),
		TraceID:   corepkg.TraceID(op.TraceID),
		Actor:     op.Actor,
		Action:    action,
		Result:    result,
		Metadata:  metadata,
	})
}

type snapshotState struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func readSnapshotCandidate(path string) (snapshotState, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshotState{}, nil
		}
		return snapshotState{}, err
	}
	if info.IsDir() {
		return snapshotState{}, fmt.Errorf("rollback snapshot target is a directory: %s", path)
	}
	if info.Size() > maxSnapshotBytes {
		return snapshotState{}, fmt.Errorf("rollback snapshot target exceeds %d bytes: %s", maxSnapshotBytes, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshotState{}, err
	}
	if !isText(data) {
		return snapshotState{}, fmt.Errorf("rollback snapshot target is not a text file: %s", path)
	}
	return snapshotState{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func captureCurrentStates(op Operation) (map[string]snapshotState, error) {
	paths := operationTouchedPaths(op)
	states := make(map[string]snapshotState, len(paths))
	for _, path := range paths {
		state, err := readCurrentState(path)
		if err != nil {
			return nil, err
		}
		states[path] = state
	}
	return states, nil
}

func readCurrentState(path string) (snapshotState, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshotState{}, nil
		}
		return snapshotState{}, err
	}
	if info.IsDir() {
		return snapshotState{}, fmt.Errorf("rollback target is a directory: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshotState{}, err
	}
	return snapshotState{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func restoreCurrentStates(states map[string]snapshotState) error {
	for path, state := range states {
		if !state.exists {
			if err := removeIfExists(path); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := state.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, state.data, mode); err != nil {
			return err
		}
	}
	return nil
}

func checkConflicts(op Operation) error {
	for _, change := range op.AffectedFiles {
		switch change.OpType {
		case OpTypeAdd, OpTypeUpdate:
			if err := requireCurrentHash(change.Path, resolveChangePath(op, change), change.AfterHash); err != nil {
				return err
			}
		case OpTypeDelete:
			_, absent, err := hashCurrentFile(resolveChangePath(op, change))
			if err != nil {
				return err
			}
			if !absent {
				return fmt.Errorf("rollback blocked: %s changed after delete", change.Path)
			}
		case OpTypeMove:
			_, oldAbsent, err := hashCurrentFile(resolveChangePath(op, change))
			if err != nil {
				return err
			}
			if !oldAbsent {
				return fmt.Errorf("rollback blocked: %s changed after move", change.Path)
			}
			if err := requireCurrentHash(change.NewPath, resolveNewChangePath(op, change), change.AfterHash); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported rollback op type %q", change.OpType)
		}
	}
	return nil
}

func requireCurrentHash(displayPath, absPath, expected string) error {
	hash, absent, err := hashCurrentFile(absPath)
	if err != nil {
		return err
	}
	if absent {
		return fmt.Errorf("rollback blocked: %s is missing", displayPath)
	}
	if hash != expected {
		return fmt.Errorf("rollback blocked: %s changed after operation", displayPath)
	}
	return nil
}

func hashCurrentFile(path string) (hash string, absent bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", true, nil
		}
		return "", false, err
	}
	return hashBytes(data), false, nil
}

func operationTouchedPaths(op Operation) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(op.AffectedFiles))
	add := func(path string) {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for _, change := range op.AffectedFiles {
		add(resolveChangePath(op, change))
		if change.OpType == OpTypeMove {
			add(resolveNewChangePath(op, change))
		}
	}
	return paths
}

func validateOperationPaths(op Operation, workspace string, writableRoots ...string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = op.Workspace
	}
	allowed, err := allowedRoots(workspace, writableRoots...)
	if err != nil {
		return err
	}
	for _, path := range operationTouchedPaths(op) {
		if !pathWithinAnyRoot(path, allowed) {
			return fmt.Errorf("rollback blocked: recorded path escapes workspace and writable roots: %s", path)
		}
	}
	return nil
}

func allowedRoots(workspace string, writableRoots ...string) ([]string, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return nil, err
	}
	roots := []string{filepath.Clean(workspace)}
	for _, root := range writableRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		roots = append(roots, filepath.Clean(abs))
	}
	return roots, nil
}

func pathWithinAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if isPathWithinRoot(root, path) {
			return true
		}
	}
	return false
}

func isPathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	candidate = filepath.Clean(strings.TrimSpace(candidate))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func normalizeTargetPath(workspace, absPath, display string) (string, error) {
	candidate := strings.TrimSpace(absPath)
	if candidate == "" {
		candidate = filepath.FromSlash(strings.TrimSpace(display))
	}
	if candidate == "" {
		return "", errors.New("rollback file path is required")
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspace, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func resolveChangePath(op Operation, change FileChange) string {
	path, err := normalizeTargetPath(op.Workspace, change.AbsPath, change.Path)
	if err != nil {
		return filepath.Clean(filepath.Join(op.Workspace, filepath.FromSlash(change.Path)))
	}
	return path
}

func resolveNewChangePath(op Operation, change FileChange) string {
	path, err := normalizeTargetPath(op.Workspace, change.NewAbsPath, change.NewPath)
	if err != nil {
		return filepath.Clean(filepath.Join(op.Workspace, filepath.FromSlash(change.NewPath)))
	}
	return path
}

func displayPath(workspace, absPath string) string {
	rel, err := filepath.Rel(workspace, absPath)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(absPath)
}

func sameWorkspace(a, b string) bool {
	aa, errA := filepath.Abs(strings.TrimSpace(a))
	bb, errB := filepath.Abs(strings.TrimSpace(b))
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func newOperationID(now time.Time) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return now.UTC().Format("20060102T150405.000000000Z")
	}
	return now.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(buf)
}
