---
name: freeze
description: |
  Restrict file edits to a specific directory for the session. Blocks Edit and Write outside the allowed path. Use when debugging to prevent accidentally "fixing" unrelated code, or when you want to scope changes to one module. Use when asked to "freeze", "restrict edits", "only edit this folder", or "lock down edits".
---

# /freeze

Restrict file edits to a specific directory for the session. Blocks Edit and Write outside the allowed path. Use when debugging to prevent accidentally "fixing" unrelated code, or when you want to scope changes to one module. Use when asked to "freeze", "restrict edits", "only edit this folder", or "lock down edits".

This demo copy keeps only the metadata that ForgeCLI actually uses at runtime.
External gstack scripts, tests, hooks, telemetry instructions, and bundled tooling were removed.