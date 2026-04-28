# Environment Variables

ByteMind reads environment variables in two categories: **install-time** (consumed by the install scripts) and **runtime** (consumed by the `bytemind` binary).

## Install-Time Variables

Passed to the install script to control how the binary is downloaded.

| Variable               | Description                            | Default                  |
| ---------------------- | -------------------------------------- | ------------------------ |
| `BYTEMIND_VERSION`     | Release tag to install (e.g. `v0.3.0`) | latest                   |
| `BYTEMIND_INSTALL_DIR` | Directory to install the binary into   | `~/.local/bin`           |
| `BYTEMIND_REPO`        | GitHub repository to download from     | `1024XEngineer/bytemind` |

**Example:**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh \
  | BYTEMIND_VERSION=v0.3.0 BYTEMIND_INSTALL_DIR=/usr/local/bin bash
```

## Runtime Variables

Read by the `bytemind` binary at startup. All runtime variables override the corresponding config file field.

| Variable                   | Overrides          | Description                                          |
| -------------------------- | ------------------ | ---------------------------------------------------- |
| `BYTEMIND_API_KEY`         | `provider.api_key` | Default API key if `api_key_env` is not set          |
| `BYTEMIND_APPROVAL_MODE`   | `approval_mode`    | Override the approval mode (`interactive` or `away`) |
| `BYTEMIND_SANDBOX_ENABLED` | `sandbox_enabled`  | Set to `true` to enable sandbox mode                 |
| `BYTEMIND_WRITABLE_ROOTS`  | `writable_roots`   | Colon-separated list of writable root paths          |
| `BYTEMIND_HOME`            | —                  | Override the `.bytemind/` base directory path        |
| `BYTEMIND_MAX_ITERATIONS`  | `max_iterations`   | Override the max tool-call iteration limit           |
| `BYTEMIND_LOG_LEVEL`       | —                  | Log verbosity: `debug`, `info`, `warn`, `error`      |

**Example — fully configured via env (no config file):**

```bash
export OPENAI_API_KEY="sk-..."
export BYTEMIND_APPROVAL_MODE="away"
export BYTEMIND_SANDBOX_ENABLED="true"
export BYTEMIND_WRITABLE_ROOTS="./src:./docs"

bytemind run -prompt "Regenerate all API docs"
```

:::tip Provider key variables
For provider-specific key variables, set `api_key_env` in `provider` config to the env var name of your choice. The `BYTEMIND_API_KEY` variable is only the fallback default.
:::

## See Also

- [Config Reference](/reference/config-reference) — full config field list
- [Configuration](/configuration) — config file format and examples
- [Installation](/installation) — `BYTEMIND_INSTALL_DIR` and `BYTEMIND_VERSION`
