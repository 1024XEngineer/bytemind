# Quick Start

This guide gets ByteMind running in about 5 minutes.

## 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

## 2. Create Config

```bash
mkdir -p .bytemind
cp config.example.json .bytemind/config.json
```

Set your provider and API key in `.bytemind/config.json`.

## 3. Start Chat Mode

```bash
bytemind chat
```

## 4. Run a First Task

Try a practical prompt:

```text
Find failing tests and fix them with minimal changes.
```

## Next

- [Installation](/en/installation)
- [Configuration](/en/configuration)
- [Chat Mode](/en/usage/chat-mode)
