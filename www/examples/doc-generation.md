# Example: Generate Documentation

This example shows how to automate documentation generation — both in interactive Chat mode for new pages and in non-interactive Run mode for CI pipelines.

## Scenario A: New CLI Command Docs (Chat Mode)

You just added a new `bytemind export` command and need user-facing docs covering usage, flags, examples, and common errors.

```text
Read the implementation of the `export` command in cmd/export.go.
Generate a user-facing markdown documentation page for it including:
- Command description and purpose
- All flags with their types, descriptions, and defaults
- 3-4 practical usage examples
- Common errors and how to fix them
Write the output to docs/cli/export.md
```

The agent reads the source code, infers intent from flag names and logic, and writes a complete doc page.

## Scenario B: API Reference from Source (Run Mode)

Automate API reference generation in CI whenever code changes:

```bash
bytemind run -prompt "\
  Read all exported functions and types in internal/api/.\
  Generate a reference page in docs/api-reference.md.\
  Format each entry with signature, description, parameters, return values, and an example.\
  Do not modify any source files.\
"
```

Add to your CI pipeline:

```yaml
- name: Regenerate API docs
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    BYTEMIND_APPROVAL_MODE: away
  run: |
    bytemind run -prompt "Regenerate docs/api-reference.md from current source code in internal/api/"
```

## Scenario C: Update Existing Docs After a Feature Change

After adding new options to an existing feature:

```text
Compare the current implementation of the configuration system in internal/config/config.go
with the existing documentation in docs/configuration.md.
Identify any undocumented fields or outdated descriptions.
Update the documentation to reflect the current implementation.
```

## Tips for Quality Documentation

:::tip Ground the agent in source code
Always ask the agent to **read the source first** before writing docs. This produces accurate docs grounded in the actual implementation rather than guesses.
:::

:::tip Constrain scope with "Do not modify"
For doc generation tasks, always include `Do not modify any source files` to prevent accidental code changes.
:::

## Expected Outcome

- Accurate, source-grounded documentation
- Task-oriented prose with concrete examples
- Flag/option tables with correct defaults
- Common error guidance

## See Also

- [Run Mode](/usage/run-mode) — CI pipeline automation
- [Chat Mode](/usage/chat-mode) — iterative doc writing
- [Tools and Approval](/usage/tools-and-approval) — `write_file` approval before writing
