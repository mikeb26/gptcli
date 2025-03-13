# Overview

gptcli is an agentic command-line interface (CLI) tool utilizing LLMs
for user chat sessions and performing automated tasks. It integrates
several capabilities, including command execution, file management,
environment variable manipulation, directory management, web
retrieval, and nested prompting.

# Key Features

- **Create conversations with LLMs:**
  - ChatGPT style conversations within the terminal
  - Persist across multiple executions for later retrieval
  - Can be archived when no longer needed
  - Current and archived conversations fully searchable

- **Automate tasks w/ Agentic Capabilities:**
  - Execute arbitrary shell commands
  - File manipulation: create, append, read, and delete files
  - Environment management: get/set current directory and environment variables
  - Retrieve content from the web

- **User Approval System:**
  - Most actions require explicit user approval. This safeguard
    ensures that potentially dangerous operations cannot be executed
    without user confirmation.

# Installation

## Building from Source

   ```bash
   git clone https://github.com/mikeb26/gptcli.git
   cd gptcli
   make
   ```

## Installing the Binary

Precompiled binaries are available for Linux. Download the appropriate binary from the [releases](https://github.com/mikeb26/gptcli/releases) page and add it to your PATH.

   ```bash
   mkdir -p $HOME/bin
   GPTCLI=$(curl -s https://api.github.com/repos/mikeb26/gptcli/releases/latest | grep browser_download_url | cut -f4 -d\")
   wget $GPTCLI
   chmod 755 gptcli
   mv gptcli $HOME/bin
   # add $HOME/bin to your $PATH if not already present
   ```

# Usage

```
gptcli - An agentic CLI based interface to LLM APIs

Available Commands:
  help                           This help screen
  config                         Set gptcli configuration (e.g. sets OpenAI key)
  upgrade                        Upgrade to the latest version of gptcli
  version                        Print gptcli's version string
  new                            Create a new thread(conversation) with GPT
  archive <thread#>              Archive a previously created thread(conversation)
  unarchive a<thread#>           Unarchive a previously archived thread(conversation)
  ls [--all]                     List available threads(conversations)
  thread <thread#>               Switch to a previously created thread
  summary [<on|off>]             Toggle thread summaries on or off
  exit                           Exit gptcli
  search <str1>[,<str2>]         Search threads for a given string(s)
  cat [<thread#>]                Show the contents of a thread(conversation)
```

## Examples

### Create a Tetris golang project

`````bash
$ ls tetris
ls: cannot access 'tetris': No such file or directory
$ gptcli
gptcli> new
Enter new thread's name: tetris
gptcli/tetris> create a new golang project implementing a basic version of the classic tetris arcade game. output the results into a new directory 'tetris'. ask me clarifying questions as needed.
gptcli: processing...
Before I begin, could you please clarify a few points about the project?

1. Would you like the Tetris game to run in the terminal (e.g., using a text-based or curses UI) or as a windowed application with a graphical interface?
2. If windowed, do you have a preference for a specific graphics library (such as SDL, Pixel, or others) for Go?
3. Are there any specific features or gameplay mechanics you want to include? For example: piece rotation, hard drops, scoring, level progression, etc.
4. Should the project include sound support or any additional enhancements beyond the basic gameplay?
5. Do you have any specific code structure or organization preferences for the project?

Once I have these details, I can create the project structure in a new "tetris" directory and proceed accordingly.
gptcli/tetris> ```
1. graphical interface
2. windowed. SDL is fine
3. rotation. no hard drops. no scoring. no levels. lets start with the basics.
4. not yet.
5. no follow best practices as appropriate.
```
gptcli: processing...
gptcli would like to 'file_create'('&{tetris/main.go package main
...<main.go contents>...
allow? (Y/N) [N]: Y
gptcli would like to 'cmd_run'('&{mkdir [-p tetris]}')
allow? (Y/N) [N]: Y
gptcli would like to 'file_create'('&{tetris/main.go package main
...<main.go contents>...
allow? (Y/N) [N]: Y
The new Tetris project is being created in the "tetris" directory with a basic SDL implementation in Go. It features a single "T-piece" that supports rotation (with bounds checking) and simple gravity. Additional details like wall kicks, hard drops, scoring, levels, and sound are omitted per your instructions.

Please let me know if you would like any modifications or additional features.
gptcli/tetris> exit
gptcli> ls
----------------------------------------------------------------------------------------------
|  Thread# |      Last Accessed |      Last Modified |            Created | Name
----------------------------------------------------------------------------------------------
|        1 | 02/26/2025 03:29pm | 10/22/2024 12:57pm | 10/22/2024 12:55pm | ipv6 cidr block
|        2 | 12/29/2024 05:18pm | 11/17/2023 07:56pm | 11/17/2023 07:31pm | github api
|        3 |      Today 03:09pm |      Today 03:09pm |      Today 03:05pm | tetris
----------------------------------------------------------------------------------------------
gptcli> exit
gptcli: quitting.
$ ls tetris/
main.go
`````

### Get Today's Top Headlines

`````bash
$ gptcli
gptcli> new
Enter new thread's name: top headlines today
gptcli/top headlines today> what are the top news headlines today?
gptcli: processing...
gptcli would like to 'url_retrieve'('&{https://news.google.com/rss?hl=en-US&gl=US&ceid=US:en [{User-Agent Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36}] GET }')
allow? (Y/N) [N]: Y
Here are a few of the top headlines extracted from today’s Google News feed:

1. <headline 1> (CNN)
2. <headline 2> (NPR)
3. <headline 3> (The New York Times)
...
gptcli/top headlines today> exit
gptcli> ls
----------------------------------------------------------------------------------------------
|  Thread# |      Last Accessed |      Last Modified |            Created | Name
----------------------------------------------------------------------------------------------
|        1 | 02/26/2025 03:29pm | 10/22/2024 12:57pm | 10/22/2024 12:55pm | ipv6 cidr block
|        2 | 12/29/2024 05:18pm | 11/17/2023 07:56pm | 11/17/2023 07:31pm | github api
|        3 |      Today 03:09pm |      Today 03:09pm |      Today 03:05pm | tetris
|        4 |      Today 03:11pm |      Today 03:11pm |      Today 03:10pm | top headlines today
----------------------------------------------------------------------------------------------
gptcli> exit
gptcli: quitting.
`````

# Contributing

Pull requests are welcome at https://github.com/mikeb26/gptcli

For major changes, please open an issue first to discuss what you
would like to change.

# License
[AGPL3](https://www.gnu.org/licenses/agpl-3.0.en.html)
