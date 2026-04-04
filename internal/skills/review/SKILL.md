---
name: review
description: |
  Review current code changes with focus on correctness, regressions, and missing tests.
when_to_use: User asks for code review, pre-merge review, or risk assessment.
---

# review

## Workflow

1. Confirm review scope (branch, directory, and files).
2. Check behavior changes before implementation details.
3. Report findings ordered by severity with precise locations.
4. List test gaps and unverified assumptions separately.

## Must Check

- Correctness and edge conditions
- Regression and compatibility risks
- Error handling and exceptional paths
- Test coverage alignment with code changes

## Output Contract

- Findings: ordered by severity with file and reasoning
- Risks: non-blocking concerns that still matter
- Verification: executed checks and recommended follow-up checks
