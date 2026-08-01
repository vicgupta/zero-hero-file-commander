# Zero Hero File Commander (zhfc)

A dual-panel, keyboard-first terminal file manager in the tradition of Norton
Commander / Midnight Commander — built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Single static binary,
no runtime dependencies.

## Features

- Two independent panels with brief/full listing modes, natural sort, hidden-file toggle
- File operations with progress and overwrite prompts: copy, move/rename, delete, mkdir, chmod
- Multi-selection: toggle, invert, select/unselect by mask
- Built-in text/hex viewer and `$EDITOR`/`$VISUAL` integration
- A real shell command line that tracks the active panel's directory
- Optional vi-style navigation (`h`/`j`/`k`/`l`/`n`/`u`) and fast filename search
- Three built-in color themes: `norton`, `nightowl`, `opencode`
- Cross-platform: macOS, Linux, Windows

See [`NC-SPECS.md`](NC-SPECS.md) for the full design specification.

## Install

Download a prebuilt binary from the [Releases](https://github.com/vicgupta/zero-hero-file-commander/releases) page, or build from source:

```sh
git clone https://github.com/vicgupta/zero-hero-file-commander.git
cd zero-hero-file-commander
go build -o zhfc .
```

## Usage

```sh
zhfc                  # open both panels in the current directory
zhfc /path/to/dir     # open both panels there
zhfc /left /right     # open each panel separately
```

Flags:

```
-left string    initial directory for the left panel
-right string   initial directory for the right panel
-theme string   color theme: norton, nightowl, or opencode
-hidden         show hidden files
-brief          start panels in brief (name-only) display mode
-vi-keys        enable h/j/k/l/n/u navigation and fast search
-version        print the version and exit
-update         download and install the latest release over this binary
```

Press `F1` inside the app for the full keyboard reference.

## Development

```sh
go build ./...
go vet ./...
go test -race ./...
```
