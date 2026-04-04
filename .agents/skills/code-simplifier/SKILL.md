---
name: code-simplifier
description: simplifies and refines code for clarity, consistency, and maintainability while preserving exact functionality. use when chatgpt needs to review, clean up, or simplify recently modified code, align code with project conventions, reduce unnecessary complexity, improve readability, or make implementation details more explicit without changing behavior.
---

Refine recently modified code for clarity, consistency, and maintainability while preserving exact functionality.

## Core behavior

Preserve behavior exactly.

- Do not change features, outputs, side effects, or observable behavior.
- Change how the code is written, not what it does.
- Do not broaden scope unless the user explicitly asks for a wider review.

Focus on code that was recently modified or touched in the current session.

- Prefer refining the code that was just written or edited.
- Avoid unrelated cleanup in untouched areas.

Prefer explicit, readable code over compact code.

- Choose clarity over brevity.
- Avoid clever one-liners when a straightforward structure is easier to read.
- Avoid nested ternary operators. Use `if/else` chains or `switch` statements for multiple conditions.

## Apply project conventions

When the repository or provided guidance includes project standards such as `CLAUDE.md`, follow them closely.

Common conventions to apply when present:

- Use ES modules with proper import sorting and explicit extensions where required.
- Prefer the `function` keyword over arrow functions when that is the project standard.
- Add explicit return type annotations for top-level functions when the project expects them.
- Follow established React component patterns with explicit `Props` types.
- Use the repository’s preferred error-handling patterns and avoid introducing unnecessary `try/catch`.
- Preserve existing naming conventions unless renaming clearly improves consistency and does not increase scope.

If no project standard is available, preserve the local style of the surrounding code while still simplifying it.

## What to improve

Look for opportunities to:

- reduce unnecessary nesting
- remove redundant abstractions
- eliminate duplicated logic when the result is clearer
- consolidate closely related logic
- improve variable, function, and component names when that materially improves readability
- remove comments that only restate obvious code behavior
- make control flow easier to follow
- keep helpful abstractions that improve organization

Do not simplify in ways that:

- reduce clarity
- make debugging harder
- combine too many concerns into one function or component
- replace readable code with dense shorthand
- remove abstractions that help structure the codebase

## Review process

Use this workflow:

1. Identify the code that was recently modified.
2. Check for project-specific conventions from repository guidance and nearby code.
3. Find the highest-value simplifications that preserve behavior.
4. Apply small, clear refinements.
5. Re-check that functionality remains unchanged.
6. Report only meaningful changes that affect readability, structure, or maintainability.

## Output expectations

When making changes:

- keep edits scoped and intentional
- preserve public interfaces unless the same interface can be expressed more clearly without behavioral change
- avoid noisy reformatting unrelated to the simplification
- prefer a few high-confidence improvements over broad cosmetic churn

When summarizing your work:

- briefly explain the significant simplifications
- mention any project conventions you applied
- note any areas you intentionally left unchanged to avoid altering behavior
