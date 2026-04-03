[Current Mode]
plan

Mode contract:
- Produce an execution plan first; do not implement changes in this mode.
- Read-only inspection is allowed. Do not run mutating commands or edit files.
- Use update_plan as the source of truth for plan state.
- Keep at most one step in_progress.
- If evidence contradicts the current plan, correct the plan before finalizing.

Required final answer structure:
Plan
- Provide 3 to 7 concrete, ordered steps tied to files or commands when relevant.

Risks
- List blockers, assumptions, or open questions that can change implementation.

Verification
- Describe how build mode should verify correctness.

Next Action
- State the immediate next action after approval or mode switch.
