# gptcli

gptcli is a terminal-based, agentic coding assistant. It wraps LLMs behind an **ncurses TUI** (thread menu + per-thread chat view) and exposes a curated set of tools (file ops, command execution, web retrieval, etc.) with a **user-approval gate** for potentially dangerous actions.

Conversations are stored as local “threads” on disk, so you can resume work across multiple executions.

## Key features

- **Ncurses terminal UI**
  - Thread list/menu with search
  - Per-thread chat view with scrollable history and multi-line input

- **Threads (persistent conversations/tasks)**
  - Stored locally and loaded on startup
  - Archive/unarchive threads
  - Search across current + archived thread
  - Multiple threads can be running (interacting with the LLM) concurrently

- **Agentic toolset**
  - Run OS-level commands
  - File manipulation: create, append, patch, read, delete
  - Directory + environment operations
  - Web retrieval with best-effort JavaScript rendering for SPA
  - Nested prompting (the agent can spawn sub-agents for decomposition of complex tasks)

- **Safety / approvals / audit**
  - Most tool actions require explicit user approval
  - Approval decisions can be persisted to an on-disk policy store
  - Optional audit log of prompts/tool usage

## Installation

### Install a prebuilt binary

Precompiled binaries are published for Linux on the
[releases](https://github.com/mikeb26/gptcli/releases) page.

Example (downloads the latest release asset named `gptcli`):

```bash
mkdir -p "$HOME/bin"

GPTCLI_URL=$(curl -s https://api.github.com/repos/mikeb26/gptcli/releases/latest \
  | grep browser_download_url \
  | cut -f4 -d\" \
  | head -n 1)

wget "$GPTCLI_URL" -O "$HOME/bin/gptcli"
chmod 755 "$HOME/bin/gptcli"

# add $HOME/bin to your PATH if not already present
```

### Build from source

```bash
git clone https://github.com/mikeb26/gptcli.git
cd gptcli
make
```

## Usage

Run:

```bash
gptcli
```

Notes:

- gptcli requires a real TTY (it uses ncurses); it is not designed for non-interactive piping.
- On first run (or when config is missing), gptcli will prompt you to choose an LLM vendor/model and enter an API key.
- When a newer version is available, gptcli may prompt to upgrade on startup.

### Key bindings (thread menu)

- Navigate: ↑/↓, PgUp/PgDn, Home/End
- Open selected thread: Enter
- New thread: `n`
- Search threads (case-sensitive): `/` (ESC exits search)
- Archive/unarchive selected thread: `a` / `u`
- Configure vendor/model/API key: `c`
- Quit: ESC

### Key bindings (thread view)

- Navigate: ↑/↓, PgUp/PgDn, Home/End
- Switch focus between history/input: Tab
- Send prompt: Ctrl-D
- Return to menu: ESC

## Configuration & storage

gptcli stores state under:

`~/.config/gptcli/`

Key files/directories:

- `.<vendor>.key` — API key for the selected vendor
- `prefs.json` — User preferences
- `threads/` — active threads (JSON)
- `archive_threads/` — archived threads (JSON)
- `approvals.json` — Persisted approval policy decisions
- `logs/audit.log` — Optional audit log

## LLM vendors & models

Supported vendors are currently: **OpenAI**, **Anthropic**, and **Google**.

Exact model names are selectable in the in-app config UI.

## Contributing

Pull requests are welcome at https://github.com/mikeb26/gptcli

For major changes, please open an issue first to discuss what you would like to change.

## License

[AGPL3](https://www.gnu.org/licenses/agpl-3.0.en.html)
