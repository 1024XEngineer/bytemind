---
name: qa-only
description: |
  Report-only QA testing. Systematically tests a web application and produces a structured report with health score, screenshots, and repro steps 鈥?but never fixes anything. Use when asked to "just report bugs", "qa report only", or "test but don't fix". For the full test-fix-verify loop, use /qa instead. Proactively suggest when the user wants a bug report without any code changes.
---

# /qa-only

Report-only QA testing. Systematically tests a web application and produces a structured report with health score, screenshots, and repro steps 鈥?but never fixes anything. Use when asked to "just report bugs", "qa report only", or "test but don't fix". For the full test-fix-verify loop, use /qa instead. Proactively suggest when the user wants a bug report without any code changes.

This demo copy keeps only the metadata that ForgeCLI actually uses at runtime.
External gstack scripts, tests, hooks, telemetry instructions, and bundled tooling were removed.