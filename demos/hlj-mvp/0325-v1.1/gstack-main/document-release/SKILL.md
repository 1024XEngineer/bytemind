---
name: document-release
description: |
  Post-ship documentation update. Reads all project docs, cross-references the diff, updates README/ARCHITECTURE/CONTRIBUTING/CLAUDE.md to match what shipped, polishes CHANGELOG voice, cleans up TODOS, and optionally bumps VERSION. Use when asked to "update the docs", "sync documentation", or "post-ship docs". Proactively suggest after a PR is merged or code is shipped.
---

# /document-release

Post-ship documentation update. Reads all project docs, cross-references the diff, updates README/ARCHITECTURE/CONTRIBUTING/CLAUDE.md to match what shipped, polishes CHANGELOG voice, cleans up TODOS, and optionally bumps VERSION. Use when asked to "update the docs", "sync documentation", or "post-ship docs". Proactively suggest after a PR is merged or code is shipped.

This demo copy keeps only the metadata that ForgeCLI actually uses at runtime.
External gstack scripts, tests, hooks, telemetry instructions, and bundled tooling were removed.