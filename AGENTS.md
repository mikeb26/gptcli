# AGENTS.md

## Project overview

This repository contains `gptcli`, a terminal-based, agentic coding assistant. It wraps LLMs behind a ncurses/stdio UI and exposes a curated toolset so agents can:

- Read, create, append, patch, and delete local files
- Run OS-level commands
- Inspect and modify the working directory and environment
- Retrieve and render web content (including JavaScript-heavy pages)
- Invoke nested LLM agents for sub-tasks

The core agent implementation is built on [CloudWeGo EINO](https://github.com/cloudwego/eino) using the React-style tool-using agent (`react.Agent`). Threads, prompts, tools, and approvals are wired together in Go.


## Setup commands

For agents that need to build, test, or lint this repo:

- Build the `gptcli` binary:
  - `make build`
- Run tests:
  - `make test`
- Run linters (if installed):
  - `make lint`

These commands assume a Go toolchain is installed and available in `PATH`. The project uses vendored dependencies (`GOFLAGS=-mod=vendor`).


## Code structure

Agents working on this repo should be aware of these key directories:

- `cmd/gptcli/` – entrypoint, ncurses UI, config & prefs handling
- `internal/llmclient/` – EINO-based agent client (`NewEINOClient`)
- `internal/tools/` – tool definitions for file/command/web/env operations
- `internal/prompts/` – system prompt and summarization prompt wiring
- `internal/threads/` – thread storage, loading, archiving, and chat flow
- `internal/types/` – thin wrappers over EINO schema, tool enums, UI interfaces
- `internal/am/` – approval policy persistence (JSON store)

If you need to change behavior in one of these areas, prefer editing the existing implementation rather than re-implementing similar logic elsewhere.


## Agent behavior & tools

### High-level behavior

- Agents in this repo are *tool-using* LLMs configured via `NewEINOClient` in `internal/llmclient/eino_client.go`.
- Supported vendors (at time of writing) are: `openai`, `anthropic`, `google`. The active vendor and model are selected via the `gptcli config` flow and `internal.DefaultModels`.
- Conversations are stored as "threads" on disk. Each thread keeps a JSON-serialized dialogue, including an initial system message from `internal/prompts/system_msg.txt`.
- When `SummarizePrior` is enabled for a user, older dialogue in a thread may be summarized via a dedicated summarization prompt before continuing the conversation, to reduce context window usage.

### Tooling model

Tools are defined in `internal/tools` and wired into the agent in `defineTools` (`internal/llmclient/eino_client.go`). Tools are identified by operation codes from `internal/types/tooltypes.go` and surfaced to the LLM with JSON schemas inferred via EINO's tool utilities.

**Core tools available to agents:**

- `cmd_run` (`RunCommandTool`)
  - Execute a single OS-level program directly (no shell). Arguments are passed as a list.
  - Do *not* invoke shell interpreters (`bash`, `sh`, `zsh`) or pass shell flags (like `-lc`) unless the human user explicitly requested shell features (pipes, redirects, `&&`, etc.).

- File operations (`CreateFileTool`, `AppendFileTool`, `ReadFileTool`, `DeleteFileTool`, `FilePatchTool`):
  - `file_create` – create or overwrite a file with given contents.
  - `file_append` – append content to an existing file.
  - `file_read` – read the full contents of a file.
  - `file_delete` – delete a file by path.
  - `file_patch` – apply a unified diff-style patch to an existing file.

- Directory and environment operations (`PwdTool`, `ChdirTool`, `EnvGetTool`, `EnvSetTool`):
  - `dir_pwd` – return current working directory.
  - `dir_chdir` – change the process working directory.
  - `env_get` – read an environment variable.
  - `env_set` – set an environment variable for the process.

- Web and rendering (`RetrieveUrlTool`, `RenderWebTool`):
  - `url_retrieve` – issue an HTTP(S) request and return raw response (status, headers, body, length). Supports custom method, headers, and body.
  - `url_render` – render a URL in a headless Chrome instance with JavaScript execution (via `chromedp`), returning the visible page text.

- Nested agents (`prompt_run`):
  - Used internally to spawn a nested agent with the same toolset but increased depth, enabling decomposition of large tasks.

**Approval & safety model:**

- All tools declare whether they require explicit user approval (`RequiresUserApproval`).
- Most state-changing tools (file writes, deletions, `cmd_run`, `dir_chdir`, `env_set`, `url_retrieve` with non-GET methods, etc.) require approval.
- Approvals are handled via `ToolApprovalUI` and persisted in a JSON policy store (`approvals.json`) under the user’s config directory.
- Tools that implement `ToolWithCustomApproval` can:
  - Offer per-file and per-directory policies for filesystem operations.
  - Offer per-URL and per-domain policies for web operations.
  - Distinguish between read-only and read/write scopes.

**Agent expectations:**

- Prefer reading relevant files (especially `AGENTS.md`) before making changes.
- Use the smallest necessary tool: prefer `file_patch` over recreating whole files when modifying existing content.
- Do not throw away or overwrite a user’s uncommitted local changes (a “dirty” working tree) without explicit confirmation.
  - Prefer to leave existing local modifications in place.
  - If your change would conflict with local modifications (e.g. would overwrite the same hunks), ask the user for confirmation before proceeding.
  - If your change does not conflict, leave the user’s local modifications untouched.
- After changes that affect build or tests, run the appropriate `make` targets and fix any issues before finishing.
- Provide clear, minimal explanations to the user, and avoid flooding them with full file contents when not necessary.


## Testing instructions

Agents should run tests when modifying Go code or tooling:

- Run the standard test suite:
  - `make test`
- Optionally, generate JUnit output (for CI contexts):
  - `gotestsum --junitfile unit-tests.xml $(TESTPKGS)`

The `Makefile` defines `TESTPKGS` to include core packages (`cmd/gptcli`, `internal/...`, `internal/am`). If you touch files under these paths, you should expect to run these tests.


## Code style and conventions

- Language: Go (modules with vendored dependencies).
- General guidelines:
  - Follow existing package boundaries (`cmd/`, `internal/`, `vendor/`).
  - Keep agent/tool behavior centralized in `internal/llmclient` and `internal/tools`.
  - Aim for small, focused tools; reuse `ToolApprovalUI` and approval policies instead of duplicating approval logic.
  - Avoid excessive defensive `nil` and empty-string checks when the value is deterministic.
    - Example: in a method with receiver `func (c *Client) Foo(...)`, `c` is necessarily non-`nil` at the callsite; do not clutter the method with `if c == nil { ... }`.
    - Example: if an internal helper is passed a string that is guaranteed by construction to be non-empty, do not add redundant `if s == "" { ... }` checks.
    - `nil` / empty checks are appropriate when the value is non-deterministic or derived from external input (e.g. user input, config files, env vars, network responses).
  - When changing public behavior of tools (e.g., adding fields to requests or responses), preserve backward compatibility with existing JSON field names.
  - Prefer **named, non-anonymous functions** for any non-trivial behavior (anything more than ~5 lines, multiple branches, or stateful logic). Avoid long inline anonymous function literals passed directly to other functions or stored in variables.
    - Example of *discouraged* style:
      - Defining a 20+ line `func(...) { ... }` literal inline inside another function body.
    - Preferred style:
      - Lift the logic into a named helper (e.g. `rebuildHistory(...)`) and call it from the caller. This keeps call sites compact and improves readability, testability, and reusability.
    - Exception:
      - Small, obviously local lambdas (~5 lines or fewer) are acceptable when they are truly trivial (e.g. a comparator passed to `sort.Slice` or a short closure that captures a couple of values).
- Testing:
  - Prefer adding or updating unit tests under `internal/...` or `cmd/gptcli/...` when you change behavior.


## Security and approvals

- Treat the filesystem and network as sensitive surfaces:
  - Always rely on `ToolApprovalUI` and `GetUserApproval` for any tool that reads/writes beyond trivial introspection.
  - File and URL approvals may be cached at file/dir or URL/domain scope, but denials are not cached.
  - Do not attempt to bypass approvals by re-implementing tools in other packages.
- When describing potentially destructive actions to the user (e.g., deleting or overwriting files), explain what will be changed in plain language before executing tools.


## PR / change guidelines for agents

When making non-trivial changes (suitable for a pull request):

1. **Understand the context**
   - Read this `AGENTS.md` and any nearby documentation or comments in the target files.
   - Inspect the relevant implementation in `internal/` and `cmd/gptcli/` instead of guessing behavior.

2. **Implement changes carefully**
   - Use the `file_*` tools to modify code.
   - Keep changes minimal and focused on the requested behavior.

3. **Run checks**
   - `make test`
   - Optionally `make lint` if `golangci-lint` is available.

4. **Describe the change**
   - Summarize what you changed in a few sentences.
   - Note any new behavior for tools, approvals, or agent configuration.


## Agent-specific tips

- Respect the user’s preferred vendor and model; these are configured through `prefs.json` and `DefaultModels`.
- Reasoning effort (applicable for some vendors/models) can be adjusted at runtime via `reasoning <low|medium|high>`. Use higher effort only when the task genuinely benefits from deeper reasoning.
- Threads live under the user’s config directory (`~/.config/gptcli/threads` by default). To inspect or debug a thread’s raw data, read the JSON files there rather than introducing new storage formats.

This file is intended as living documentation for agents. If you find yourself needing extra project-specific guidance that isn’t covered here, prefer updating `AGENTS.md` alongside your code changes.
