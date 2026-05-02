# SubAgent MVP3/MVP4 Handoff

This document defines the remaining scope after MVP2.

## MVP2 Baseline (Already Landed)

- `delegate_subagent` tool contract and registry wiring.
- Preflight gateway:
  - task/agent/mode validation
  - tool narrowing (`parent visible ∩ definition tools ∩ parent allowed - denied`)
  - recursive delegation removal (`delegate_subagent` always denied)
  - timeout/output/isolation normalization and validation
- Agent-side runtime task envelope for delegation:
  - structured result normalization and contract checks
  - stable status/error mapping
  - runtime metadata (`invocation_id`, `effective_tools`, `effective_toolset_hash`, timeout fields)
- Current boundary: subagent execution body is still a placeholder and returns `subagent_not_implemented`.
- Prompt/runtime exposure for available subagents and slash surfaces (`/agents`, builtin `/review`, `/explorer`).

## MVP3 Scope (Execution Isolation + Worktree Lifecycle)

Primary goal: make delegated execution safe for write-capable subtasks.

- Implement worktree lifecycle manager for delegated runs:
  - create per-subagent worktree when required
  - cleanup on success/failure
  - startup reconcile for orphan worktrees
- Enforce write-tool isolation:
  - if effective tools contain write-capable tools, require worktree isolation
  - if worktree creation fails, fail before child execution starts
- Add lifecycle observability:
  - creation success/failure
  - cleanup success/failure
  - orphan count and reconcile cleanup count
- Keep parent workspace untouched by child write operations.

## MVP4 Scope (Background Delegation + Approval Consistency)

Primary goal: enable `run_in_background=true` safely and deterministically.

- Enable async path for `delegate_subagent`:
  - return stable `task_id` for queued/accepted/running states
  - parent result contract must stay stable (`findings`/`references` always arrays)
- Add hard preconditions:
  - if `task_output` or `task_stop` tool is unavailable, reject background delegation
- Enforce pre-approval consistency for background runs:
  - bind snapshot to effective toolset (including hash)
  - reject on capability drift (no interactive escalation in background path)
- Define deterministic parent behavior for child terminal states and timeout/cancel paths.

## Required Go/No-Go Gates

- Security:
  - no recursive delegation
  - no toolset privilege expansion
  - no write-capable delegation without isolation
- Reliability:
  - runtime task quota release on all terminal states
  - worktree cleanup/reconcile is idempotent
- Contract:
  - `DelegateSubAgentResult` schema remains stable
  - error code semantics remain stable and test-covered
- Background:
  - reject fast when async prerequisites are missing
  - strict permission snapshot matching

## Suggested Commit Slicing for Next Owner

1. `feat(subagents): add worktree manager and reconcile lifecycle`
2. `feat(subagents): enforce write-tool isolation gating in preflight/dispatch`
3. `test(subagents): add worktree lifecycle and reconcile safety matrix`
4. `feat(agent): enable delegate_subagent background runtime path`
5. `feat(agent): enforce background preapproval snapshot and capability hash checks`
6. `test(agent): add async delegate_subagent status/error/contract matrix`
