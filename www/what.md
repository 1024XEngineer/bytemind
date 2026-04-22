# What is ByteMind?

ByteMind is a **terminal-first AI Coding Agent** written in Go.

It lets you collaborate with large language models directly inside your code repository — reading files, making changes, running searches, and executing commands — without ever leaving your terminal.

## The Problem It Solves

Modern AI coding tools often require a browser, a cloud IDE, or a proprietary editor extension. ByteMind takes a different approach:

- **Stay in your terminal.** No context switching between editor and browser.
- **Your code stays local.** ByteMind operates on your filesystem directly; nothing is uploaded to a third-party service beyond the API calls you configure.
- **Full task completion.** Not just code suggestions — ByteMind can read, modify, search, and verify changes in a multi-step loop until the task is done.

## How It Works

You start a chat session in your repository:

```bash
bytemind chat
```

ByteMind assembles context from your workspace, sends it to your configured LLM, and enters a tool loop:

1. The model reads your files, runs searches, and proposes changes.
2. Actions that modify the filesystem or execute shell commands require your approval.
3. The loop continues until the task is complete or the iteration budget is reached.
4. Everything is saved to a persistent session you can resume later.

## Design Principles

| Principle | What it means |
|-----------|--------------|
| **Local first** | Runs on your machine; no mandatory cloud dependency |
| **Terminal first** | Native CLI and TUI; no GUI required |
| **Provider agnostic** | Works with OpenAI-compatible APIs and Anthropic |
| **User controlled** | Approval gates and iteration budgets keep you in charge |
| **Recoverable** | All sessions are persisted and resumable |

## Next Steps

- [Install ByteMind →](./installation)
- [Explore all features →](./features)
