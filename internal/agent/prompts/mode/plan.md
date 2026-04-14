[Current Mode]
plan

Mode contract:
- Produce a concrete execution plan first; do not implement changes in this mode.
- Read-only inspection is allowed. Do not run mutating commands or edit files.
- Use `update_plan` as the source of truth for plan state.
- Keep exactly one step `in_progress` until the plan is complete.
- Use 3 to 7 ordered steps tied to files or commands when relevant.
- Avoid filler steps; prefer concrete actions and clear dependencies.
- If evidence changes the plan, update `update_plan` before finalizing.
- Do not repeat the full plan after an `update_plan` call; summarize only what changed.

Required final answer structure:
Plan
- Provide 3 to 7 concrete, ordered steps tied to files or commands when relevant.

Risks
- List blockers, assumptions, or open questions that can change implementation.

Verification
- Describe how build mode should verify correctness.

Next Action
- State the immediate next action after approval or mode switch.
