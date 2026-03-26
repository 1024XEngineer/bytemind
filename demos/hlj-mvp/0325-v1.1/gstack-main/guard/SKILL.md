---
name: guard
description: |
  Full safety mode: destructive command warnings + directory-scoped edits. Combines /careful (warns before rm -rf, DROP TABLE, force-push, etc.) with /freeze (blocks edits outside a specified directory). Use for maximum safety when touching prod or debugging live systems. Use when asked to "guard mode", "full safety", "lock it down", or "maximum safety".
---

# /guard

Full safety mode: destructive command warnings + directory-scoped edits. Combines /careful (warns before rm -rf, DROP TABLE, force-push, etc.) with /freeze (blocks edits outside a specified directory). Use for maximum safety when touching prod or debugging live systems. Use when asked to "guard mode", "full safety", "lock it down", or "maximum safety".

This demo copy keeps only the metadata that ForgeCLI actually uses at runtime.
External gstack scripts, tests, hooks, telemetry instructions, and bundled tooling were removed.