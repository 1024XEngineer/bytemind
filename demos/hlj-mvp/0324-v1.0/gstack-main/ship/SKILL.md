---
name: ship
description: |
  Ship workflow: detect + merge base branch, run tests, review diff, bump VERSION, update CHANGELOG, commit, push, create PR. Use when asked to "ship", "deploy", "push to main", "create a PR", or "merge and push". Proactively suggest when the user says code is ready or asks about deploying.
---

# /ship

Ship workflow: detect + merge base branch, run tests, review diff, bump VERSION, update CHANGELOG, commit, push, create PR. Use when asked to "ship", "deploy", "push to main", "create a PR", or "merge and push". Proactively suggest when the user says code is ready or asks about deploying.

This demo copy keeps only the metadata that ForgeCLI actually uses at runtime.
External gstack scripts, tests, hooks, telemetry instructions, and bundled tooling were removed.