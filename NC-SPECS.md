# Norton Commander Clone — Technical Specification (Go)

**Status:** Draft v1.0
**Author:** opencode (researched & drafted for the `norton-commander` project)
**Date:** 2026-08-01
**Language:** Go
**Interface:** Text User Interface (TUI) in a terminal

---

## 1. Executive Summary

This document specifies a modern, cross-platform clone of **Norton Commander (NC)** —
the prototypical "orthodox file manager" (OFM) written by John Socha and first released
by Peter Norton Computing in May 1986. The clone reproduces NC's signature dual-panel,
keyboard-first design while targeting contemporary terminals and operating systems.

The target is a single static Go binary with no runtime dependencies, an authentic
NC-style screen layout, the classic F1–F10 function-key vocabulary, a built-in command
line, viewer, editor, file search, multi-selection, and an extensible architecture.

The reference "spiritual" descendants for interaction details are Midnight Commander
(mc) and the classic NC 5.0/5.51 for DOS, whose key bindings are preserved almost
one-to-one in this specification.

---

## 2. Background & Historical Reference

### 2.1 Norton Commander timeline (why we build this)

| Version | Year | Notable changes |
|---|---|---|
| NC 1.0 | 1986 | Original, by John Socha (working title "Visual DOS"). F1–F10 keys, dual panels, mini-help bar, half-screen command line, rudimentary viewer/editor, one-screen help. |
| NC 2.0 | 1988 | Added NCD-style directory tree (`Alt+F10`), `treeinfo.ncd` cache concept, better sorting. |
| NC 3.0 | 1989 | Hypertext help, quick-view mode (viewer minimized into panel), integrated file viewers as mini-plugins, client-server comms over serial port, screen saver. First version with mouse support. Peak of the DOS line. |
| NC 4.0 | 1991 | File attributes command, better viewer/editor integration. |
| NC 5.0 | 1994 | Archive VFS (ZIP), Search VFS ("panelize"), directory synchronization (`Ctrl+F8`), label disk, invert/restore selection, customizable panel filters, user menu file (`NC.MNU`). |
| NC 5.51 | 1998 | Long-filename support (TSR-assisted on real mode), disk housekeeping/cleanup, file splitter, regex search, drag & drop. Final DOS version. |

NC defined the **orthodox file manager paradigm**: two side-by-side file panels, a
command line, a function-key bar, keyboard-first operation, and an "enter a directory /
exit to parent" navigation model.

### 2.2 The OFM paradigm we reproduce

- **Two panels always visible** — not a mode, not a toggle. Left and right panes each
  display a directory listing, the directory tree, a quick-view of the file under the
  cursor, or the contents of the other panel.
- **Active panel concept** — one panel is "active"; the path header is highlighted.
  `Tab` toggles which panel is active.
- **Operation direction model** — e.g. `F5` copies from the *active* panel to the
  *passive* panel's current directory.
- **Function-key vocabulary** — F1..F10 map to the same operations as NC; the bottom
  bar always shows the current mapping. `Ctrl`/`Alt` extend the bar with additional
  commands.
- **Command line at the bottom** — a real shell prompt that tracks the active panel's
  directory; ordinary characters typed in panel mode go into the command line.
- **Selection via `Insert`** — plus keypad `+`/`-`/`*` wildcard selection and inversion.

---

## 3. Goals & Non-Goals

### 3.1 Goals

1. Faithful NC look-and-feel: dual panels, top menu bar, bottom function-key bar,
   classic colors (default blue DOS scheme).
2. Complete core file operations: copy, move/rename, delete, mkdir, chmod/chown.
3. Built-in **viewer** (text + hex) and **editor** with search.
4. **Command line** with shell passthrough and NC-style command entry.
5. Multi-file selection with wildcard selection and inversion.
6. Directory **tree** view and **quick view** panel modes.
7. **Find files** dialog with panelize-to-results.
8. **Directory synchronization** between panels (NC5 `Ctrl+F8`).
9. Configurable **key bindings** and persisted configuration (layout, sorting, filters).
10. Cross-platform: macOS, Linux, Windows (Tier 1) with a single static binary.
11. Mouse support for the function-key bar, panel clicks, and double-click open.

### 3.2 Non-Goals (v1)

- GUI (Electron/Fyne/GTK) — this is a TUI.
- Network file systems (SFTP/FTPS/S3) in v1 — the architecture allows pluggable backends.
- Archive writing (compression) in v1; read-only archive **browsing** (as VFS) is a
  stretch goal.
- Full Norton Desktop replacement features (disk formatting, network utilities).
- Multi-instance client-server communication (NC 3.0 serial feature).

---

## 4. Technical Stack

### 4.1 Recommended libraries

| Concern | Recommended | Rationale |
|---|---|---|
| TUI framework | **Bubble Tea** (`charm.land/bubbletea/v2`) | Elm-style Model/Update/View, mature (40k+ stars), used by GitHub & GitLab CLIs, excellent async (`tea.Cmd`) story for non-blocking file I/O, first-class keyboard/mouse events, alt-screen. v2.0 shipped Feb 2026 with declarative `tea.View`, key press/release split, Kitty keyboard protocol. |
| Styling | **Lip Gloss** (`charmbracelet/lipgloss/v2`) | Width-aware layout (respects CJK double-width), borders, colors; composes with Bubble Tea. |
| Components | **Bubbles** (`charmbracelet/bubbles`) | `viewport`, `textinput`, `list`, `progress` for dialogs, editor area, and copy progress bars. |
| Input events | Bubble Tea built-in + tcell-compatible parsing | High-fidelity keys incl. `F1..F24`, `Ctrl+Alt`, keypad, mouse. |
| Config | `TOML` via `BurntSushi/toml` | Human-editable keymaps + options (proven by pelorus). |
| Filesystem events | `fsnotify` | Optional live-refresh of panel on external changes. |
| Text search | `regexp` (stdlib) for v1 | NC5.51 used regex search; add ripgrep integration later. |
| Trash (safe delete) | platform: `os/exec` to trash CLI / custom | Optional "delete to trash" (NC Windows `Shift+F8`). |
| Build | `go build` with `CGO_ENABLED=0` | Single static binary, no runtime deps. |

**Alternatives considered:**

- **tview** (`rivo/tview`): mature, widget-rich, imperative; used by k9s, gh CLI, and
  the existing Go VC clone (`feherk/vc`). Good fallback if team prefers imperative
  widgets over Elm-style architecture.
- **tcell** (`gdamore/tcell`): low-level cell rendering; maximum control but maximum
  boilerplate. Only if we abandon a framework.
- **termbox-go / tui-go**: deprecated or low-level; not recommended.

**Decision rationale:** Bubble Tea's `Cmd`/goroutine model is the cleanest fit for the
long-running, non-blocking copy/move operations NC requires, and its renderer handles
the fast full-screen redraws a file manager needs. Lip Gloss's `Width()` handles CJK
filenames correctly (a hard requirement for real-world terminal file managers).

### 4.2 Go version & modules

- Go 1.22+ (1.23+ recommended).
- Module path: `github.com/<user>/norton-commander` (placeholder).
- Zero CGO; `GOOS`/`GOARCH` cross-compile targets: darwin/amd64+arm64, linux/amd64+arm64,
  windows/amd64.

---

## 5. Screen Layout

Layout in NC terms, top to bottom. Default terminal minimum: **80 cols x 25 rows**.

```
┌─ Left Panel ────────────────────────────┐┌─ Right Panel ───────────────────────────┐
│  C:\                                        │  C:\DATA                                    │
│ ┌Name         Ext  Size   Date    Attr ┐   │ ┌Name         Ext  Size   Date    Attr ┐     │
│ │..            <DIR>  08-01-96  0:00   │   │ │..            <DIR>  08-01-96  0:00   │     │
│ │NC           <DIR>  08-01-96  0:00   │   │ │report.txt    TXT  12,345 08-01-96  9:00 │   │
│ │README   TXT   2,048 07-30-96 11:22  │   │ │data.csv      CSV  98,765 07-31-96 3:11 │     │
│ │setup   EXE 112,640 06-01-96 18:00   │   │ │...                                          │
│ ...                                      │ │ ...                                          │
│ └───────────────────────────────────────┘   │ └──────────────────────────────────────┘     │
│  C:\> _                                       │                                             │
│ ┌F1 Help┐┌F2 Menu┐┌F3 View┐┌F4 Edit┐┌F5 Copy┐┌F6 Move┐┌F7 MkDir┐┌F8 Del┐┌F9 Menu┐┌F10 Quit┐ │
└──────────────────────────────────────────────┴──────────────────────────────────────────┘
```

### 5.1 Elements

| # | Element | Notes |
|---|---|---|
| 1 | **Top menu bar** | `Left  Files  Commands  Options  Right` (NC-style). Only visible when `F9` pressed (pull-down) or configured `Options → Always show menu`. |
| 2 | **Panel header** | Active panel header uses a highlight/inverse background. Shows current path. May include free-space info. |
| 3 | **Panel body** | File listing. Column layout depends on display mode (see §6.2). Cursor bar on current row. Selected files shown in a distinct color (NC: yellow). |
| 4 | **Divider / panel resize** | A column between panels. `Ctrl+F1`/`Ctrl+F2` show/hide panels; `Alt+F1`/`Alt+F2` change drive. `<>` or `Ctrl+Left/Right` resize the split. |
| 5 | **Command line** | `C:\>_` prompt. Tracks the active panel path. Full command history (NC keeps a recent-commands ring; `Ctrl+E` recalls, arrows scroll history). |
| 6 | **Function-key bar** | Bottom two rows: `F1 Help F2 Menu F3 View F4 Edit F5 Copy F6 Move F7 MkDir F8 Del F9 Menu F10 Quit`. `Ctrl`/`Alt` press reveals extended bar (see §7.3). |

---

## 6. Panels

### 6.1 Panel state model

Each panel maintains independent state:

```
PanelState {
  path: string                // current directory (absolute)
  entries: []FileEntry        // directory listing
  cursor: int                 // index of cursor row
  selected: map[string]bool   // relative path -> selected flag
  filter: string              // e.g. "*.txt;*.md" (NC filter)
  sortMode: Name|Ext|Size|MTime|Uname|Gname   // + direction
  displayMode: Brief|Full|Tree|QuickView|Info|Listing       // see §6.2
  drive: string               // logical root (C:, /, etc.)
  viewVersion: int            // bump to force redraw
}
```

### 6.2 Display modes (per panel, via `Left`/`Right` menu)

| Mode | Description |
|---|---|
| **Brief** | Filename only (NC default on first run). |
| **Full** | Name, Extension, Size, Date, Time, Attributes. |
| **Tree** | Directory tree of the drive; `Alt+F10` also opens a full-screen tree. |
| **Quick View** | Shows content of the file under the *other* panel's cursor (viewer minimized in panel). Read-only. |
| **Info** | Drive/path info, free space, selected files count/size, system info. |
| **Listing** (optional) | Panel mirrors the other panel's directory (NC "Listing" mode). |

Mode is selectable independently per panel and per tab (tabs are v2, see §12).

### 6.3 Sorting

- Sort by **Name, Extension, Size, Last Modified, User, Group**; each asc/desc.
- Directories sort before files by default (configurable).
- Case-insensitive natural sort (numerical chunks) is the default for names.

### 6.4 Filters

- Panel filter string (e.g. `*.go`, `*.txt;*.md`, `!*.tmp`). Supports `*` and `?`
  wildcards, `;`-separated includes, `!`-prefixed excludes.
- `Ctrl+F11` toggles filter input on the active panel.

### 6.5 Navigation keys

| Key | Action |
|---|---|
| `Up` / `Down` | Move cursor |
| `PageUp` / `PageDown` | Scroll by page |
| `Home` / `End` | First / last row |
| `Enter` | If dir → enter it; if file → open per associations or run if executable |
| `..` (row) + `Enter`, or `Backspace` | Go to parent directory |
| `Left` / `Right` (tree mode) | Collapse / expand node |
| `Ctrl+R` | Re-read directory (`F17`/`S-F5` in some clones) |
| `Alt+F1` / `Alt+F2` | Change drive, left / right panel |
| `Ctrl+F1` / `Ctrl+F2` | Hide/show left / right panel |
| `Ctrl+U` | Swap panel contents |
| `Ctrl+\` | Jump to drive root |
| `Ctrl+Enter` | Insert current file name into command line |
| `Ctrl+O` | Toggle to/from full-screen shell (suspend/restore) |
| `Ctrl+L` | System/drive info dialog |

---

## 7. Function Keys & Extended Bars

### 7.1 Core F-key vocabulary (NC canonical)

| Key | Action | Notes |
|---|---|---|
| `F1` | Help | Hypertext help; one-screen hotkey reference |
| `F2` | User Menu | Custom menu from `NC.MNU`/config |
| `F3` | View | Open viewer (text/hex) |
| `F4` | Edit | Open editor (or `$EDITOR`) |
| `F5` | Copy | Active panel → passive panel path (dialog with options) |
| `F6` | Move/Rename | Same target model as copy |
| `F7` | Make Directory | Prompt for new dir name |
| `F8` | Delete | With confirmation dialog; wildcard-aware |
| `F9` | Top Menu | Show pull-down menus |
| `F10` | Quit | Exit |

### 7.2 Shift/F/Ctrl/A It extensions

- `Shift+F3` — external viewer for selected file type.
- `Shift+F4` — create & edit new file.
- `Shift+F5`/`Shift+F6` (optional) — hard link / symlink (mc-style).
- `Shift+F8` — delete to trash (optional).
- `Ctrl+F5` — copy within the same panel directory (target = same dir).
- `Ctrl+F8` — synchronize directories.
- `Ctrl+F11` — panel filter input.
- `Ctrl+F12` — split file (optional).

### 7.3 Bottom-bar modes

- **Normal:** F1..F10 as above.
- **Ctrl-pressed:** bar shows `Ctrl+` commands (reread, filter, sync, swap, find, etc.).
- **Alt-pressed:** bar shows `Alt+` commands (change drive L/R, tree, etc.).

---

## 8. Command Line

- Located above the function-key bar. Prompt = active panel path + `> _`.
- Regular printable characters typed in panel mode are echoed to the command line.
- `Enter` executes via the system shell:
  - Unix: `sh -c "<cmd>"` (or `$SHELL -c`); Windows: `cmd /C <cmd>`.
  - Runs in a spawned child; terminal switches to normal (non-alt) screen while it runs,
    then returns to NC (NC "swapping" behavior). Bubble Tea achieves this via
    `tea.Exec` / alternate-screen suspend.
- `Ctrl+E` re-inserts last command; `↑`/`↓` scroll command history; `Ctrl+Enter`
  inserts current file name.
- `cd`, `pushd`, drive letters (`D:`), and `..` typed here update the active panel path
  (NC behavior).
- Command history persisted in config file (last 100).

---

## 9. File Operations

### 9.1 Copy (F5)

1. Target = passive panel current directory (pre-filled in dialog, editable).
2. Dialog options:
   - Copy to: `<path>`
   - Include subdirectories (`tree copy`)
   - Use selected files only / current file only
   - Overwrite policy: Ask / Skip / Overwrite / Rename
3. Multi-file: copies all selected entries.
4. Progress: per-file + aggregate progress bar (Bubbles `progress`), ETA, speed.
5. Non-blocking: run in a `tea.Cmd` goroutine; UI stays responsive; cancellation via
   `Esc` or "Cancel" button.
6. Preserve permissions and mtime. Symlinks copied as links (configurable).
7. Name-collision dialog shows file sizes and offers Rename.

### 9.2 Move / Rename (F6)

- Move behaves as copy + verified delete. Within same directory, moves are cheap
  renames.
- Rename dialog supports batch rename via wildcards (e.g. `*.TXT` → `*.BAK`).
- Directory move supports `tree move` (move subdirs).

### 9.3 Delete (F8)

- Confirmation dialog listing count + total size. Default `F8` deletes immediately on
  second confirmation; NC uses a confirm dialog.
- Recursive delete for directories; wildcard-aware (`keypad *` then `F8`).
- Optional trash support on macOS (`.Trash`), Linux (`gio trash`/`trash-cli`), Windows
  (Recycle Bin) — **delete-to-trash default ON**, hard delete via `Shift+F8`.

### 9.4 Make Directory (F7)

- Prompt for name; supports nested path (`a/b/c`); creates parents recursively.
- `Shift+F7` = create + immediately enter.

### 9.5 Attributes (F11 / Ctrl+A)

- chmod (numeric + symbolic), chown (user/group picker), toggle read-only/hidden.
- Show current attrs, sizes, mtime.

### 9.6 Size calculation (`Space` on selection, `Alt+F9`)

- Compute total size of selected entries recursively; show in status line.

### 9.7 Operation engine design

All destructive/bulk operations go through an **Operation** interface so the UI layer
never touches the filesystem synchronously:

```
type Operation interface {
    Name() string
    Run(ctx, progress chan Progress) error   // runs in goroutine
    Cancel()
    Undo() error        // where feasible (v2)
}
```

Progress events flow back as `tea.Msg` values; the model renders the status bar.

---

## 10. Multi-Selection Model

| Key | Action |
|---|---|
| `Insert` | Toggle selection on cursor row, advance cursor |
| `Space` | Toggle selection, advance cursor (optional; used for size calc in NC) |
| Keypad `+` | Select-by-mask dialog (e.g. `*.txt`) |
| Keypad `-` | Unselect-by-mask dialog |
| Keypad `*` | Invert selection (if none selected → select all) |
| `Ctrl+T` (optional) | Select all |
| `Ctrl+\` (optional) | Unselect all |

- Selected rows render highlighted (NC: yellow text on default bg).
- Operations (F5/F6/F8) operate on the selection when non-empty, else the cursor file.
- Selection is preserved across re-reads by name where possible.
- Status line shows `N files selected, total size`.

---

## 11. Viewer & Editor

### 11.1 Viewer (F3)

- Modes: **Text** (auto-detected), **Hex**, **Binary-safe** (detects non-text).
- Text: `PageUp/Down`, arrows, `Home/End`, `Ctrl+F` search, `F` toggles hex/text
  (NC: `F4` in viewer toggles to editor).
- Hex: address column, offset highlight, search for byte string.
- Quick view mode (§6.2): renders inside the panel at reduced height.
- Viewer handles arbitrarily large files by streaming/seek rather than loading fully.
- Encoding: UTF-8 first; detect UTF-16/GBK where cheap (golang.org/x/text); fallback
  to raw bytes with `?` placeholders.

### 11.2 Editor (F4)

- Full-screen text editor.
- Cursor movement, insert/overwrite, block copy/move/delete (`Ctrl+K`-style or
  nc-style), search & replace, undo/redo.
- `F2` save, `F9` menu, `F10` exit (prompt save if modified).
- Optionally delegate to `$EDITOR` (`Shift+F4` create-and-edit, or config `editor=...`).
- v1 target: functional (not feature-complete) — cursor, insert, delete, search,
  save. v2: undo/redo, block ops.

### 11.3 File type handling

- Extension association table (NC `Extensions file`):
  `ext → program + argument template` (`%d`=drive, `%p`=path, `%f`=file, `%t`=dir).
- `Enter` on a file with an association launches the program with the file.
- Built-in defaults: `.txt/.md/.log/.go/.c/...` → internal viewer/editor.

---

## 12. Menus (Top Pull-Down)

Menu bar: `Left | Files | Commands | Options | Right` (also `Help` in some builds).

### 12.1 `Left` / `Right` (mirror)
- Brief / Full / Tree / Quick View / Info / Listing
- Sort by: Name / Ext / Size / Time / User / Group
- Re-read (`Ctrl+R`)
- Change drive (`Alt+F1`/`Alt+F2`)
- Filter (`Ctrl+F11`)
- Link (v2)

### 12.2 `Files`
- Copy `F5`, Move `F6`, MkDir `F7`, Delete `F8`, Edit `F4`
- Attributes `Ctrl+A`
- Pack/Unpack (archive; v2 write)
- Split / Join (optional)

### 12.3 `Commands`
- Find file... (`Alt+F7`)
- Sync directories... (`Ctrl+F8`)
- Compare directories (new: differs from sync; shows only differences)
- Compare files (binary/diff two files)
- Directory tree (`Alt+F10`)
- Swap panels (`Ctrl+U`)
- Terminal / Run shell (`Ctrl+O`)
- System info (`Ctrl+L`)
- Extension file edit... (associations)
- Menu file edit... (user menu)
- Command history

### 12.4 `Options`
- Configuration... (dialog: colors, sort, auto-save setup, show hidden, show menu bar,
  save options on exit, fast search, panel default modes, delete-to-trash)
- Screen colors... (palette picker; default = classic NC blue)
- Save setup on exit (toggle)
- Auto-save current layout to config

---

## 13. Search (Find Files)

- `Alt+F7` opens find dialog:
  - File mask (wildcards), start directory (default = active panel path), recursive
  - Search by content (substring or regex), by size range, by date range
  - Exclude masks
- Results list with `F3` view, `Enter` goto (navigate panel to containing dir),
  and **Panelize** (`Ctrl+P`) → pushes results into a read-only search panel
  (NC5 Search VFS concept; operations on panelized list allowed read-only for v1).
- Large result sets streamed; cancellation via `Esc`.

---

## 14. Directory Synchronization (Ctrl+F8)

- Compares the two panels recursively.
- Options: by size; by size+mtime; by content (hash); filename-only.
- Result view: rows `[→ copy to right] [← copy to left] [↔ overwrite both] [= skip]`.
- Batch apply after review; dry-run preview.
- Reuses the operation engine with progress.

---

## 15. Archives (Stretch / V2)

- Read-only **archive VFS**: enter `.zip`, `.tar`, `.tar.gz`, `.bz2`, `.xz`, `.7z` as
  virtual directories.
- Pure-Go via `archive/zip`, `archive/tar`, `compress/gzip|bzip2|xz`, `mholt/archives`.
- Operations inside archive panel: extract to other panel (`F5`).
- v2: writing archives (pack).

---

## 16. Configuration & Persistence

### 16.1 Config file

- Path (XDG): `~/.config/norton-commander/config.toml`
- Windows: `%APPDATA%\norton-commander\config.toml`

Sections:

```toml
[options]
show_hidden = false
show_menu_bar = false
save_setup_on_exit = true
delete_to_trash = true
confirm_delete = true
confirm_overwrite = "ask"          # ask|skip|overwrite
editor = ""                        # empty => internal editor
shell = ""                         # empty => $SHELL / cmd
fast_search = true
auto_save_layout = true
minimum_screen_width = 80

[colors]
scheme = "norton"                  # norton | midnight | tokyonight | custom

[panels]
left_display = "brief"
left_sort = { by = "name", dir = "asc" }
left_filter = ""
right_display = "full"
right_sort = { by = "time", dir = "desc" }
right_filter = ""

[keymap]
"f5" = "copy"
"insert" = "toggle_select"
# ... every action remappable

[associations]
"md"  = { program = "glow", args = "%f" }
"png" = { program = "chafa", args = "%f" }
"txt" = { program = "", args = "" }   # internal viewer
```

### 16.2 Runtime state

- Auto-save layout (panel paths/modes/sorts) on exit and on change (debounced).
- Command history: `~/.local/share/norton-commander/history` (or XDG state dir).
- Jump/bookmark list (frecency-scored, v2) in state dir.

### 16.3 Keymap

- Every action has a symbolic name; `[keymap]` maps key chords → action names.
- Defaults mirror NC exactly; users override only what they change.
- Validation at load; unknown bindings logged and ignored.

---

## 17. Architecture

### 17.1 Package layout (Go)

```
cmd/norton-commander/        main.go — CLI entry, flags, program bootstrap
internal/
  app/                       root Bubble Tea model: state, message routing, views
    model.go                 Model struct + Init/Update/View
    msgs.go                  all tea.Msg types (typed event bus)
    keys.go                  global vs context key dispatch (layered)
  panel/                     panel model, navigation, selection, display modes
    panel.go                 PanelState, cursor, selection
    list.go                  rendering brief/full/tree/quickview/info
    sort.go                  comparators
    filters.go               mask matching
  backend/                   storage abstraction
    backend.go               Backend interface (ReadDir, Stat, Copy, Move, Mkdir, ...)
    local.go                 os.FS implementation
    vfs.go                   archive & search VFS (v2)
    copy.go                  copy engine (streams, progress, perms)
    move.go delete.go mkdir.go
  viewer/                    F3 viewer (text/hex) + quick view
  editor/                    F4 editor (text buffer, search, save)
  commandline/               prompt, history, execution (tea.Exec)
  menus/                     top-bar model, pull-down menu rendering/navigation
  dialogs/                   reusable input/confirm/list dialogs (Bubbles)
  search/                    find files engine + panelize
  sync/                      directory comparison & sync
  config/                    TOML load/save, keymap validation, defaults
  colors/                    schemes (norton default, midnight, etc.)
  util/                      human size, time fmt, natural sort, width helpers
```

### 17.2 Layered key dispatch

- **Root model** handles global keys first (quit, help, screen-switch).
- Then active **sub-view** (dialog/menu/editor/viewer) if any.
- Then **panel context** keys.
- Unmatched printable keys → command line.

This layered approach (root → overlay → panel → commandline) mirrors the SprintOS /
Bubble Tea best-practice pattern and keeps conflicts explicit.

### 17.3 Backend interface

```
type Backend interface {
    ReadDir(path string) ([]Entry, error)
    Stat(path string) (Entry, error)
    Mkdir(path string, perm) error
    Remove(path string) error
    RemoveAll(path string) error
    Copy(ctx, src, dst, overwrite) (Progress, error)
    Move(ctx, src, dst) error
    Rename(old, new string) error
    Chmod/Chown(path, ...) error
    Open(path) (io.ReadSeekCloser, error)
    Create(path) (io.WriteCloser, error)
    Watch(path, cb) (cancel, error)   // fsnotify, optional
}
```

The UI never imports `os` directly — enabling tests with an in-memory fake backend
and later SFTP/archive backends.

### 17.4 Rendering model

- Bubble Tea v2 `tea.View` (declarative) composed from:
  - `menubar.View` → panels (Horizontal layout, `lipgloss.JoinHorizontal`) →
    commandline → function-bar.
- Panel view functions are **pure**: `(PanelState, width, height) → View`.
  This makes rendering unit-testable without a terminal.
- Handle `tea.WindowSizeMsg` from day one (§17.5), since panels must reflow.

### 17.5 Resize handling

- Minimum size enforcement: warn banner below 80x25; panels degrade gracefully
  (brief mode on very narrow terminals).
- On resize: recompute panel widths (percentage split), clamp scroll positions,
  re-render command line.

---

## 18. Non-Blocking I/O & Concurrency

- Every filesystem operation runs inside `tea.Cmd` goroutines; results and progress
  return as messages.
- A **global operation queue** serializes destructive ops; independent reads may run
  concurrently.
- Cancellation: context cancellation propagated; `Esc` cancels current op.
- No file operation ever blocks the UI thread — verified by test harness (see §20).

---

## 19. Platform Notes

| Platform | Considerations |
|---|---|
| macOS | Alt/Option-key chords need `ESC`-prefix handling; `$EDITOR` typically `vim/nano`; trash = `~/`.Trash via `os` move. |
| Linux | `Alt` chord support; trash via `gio trash` or `trash-cli`; truecolor terminals common. |
| Windows | cmd vs PowerShell execution; drive letters in `Alt+F1/F2`; hidden/system attribute mapping; ANSI support via Windows Terminal. |

---

## 20. Testing Strategy

1. **Unit tests** (stdlib `testing`):
   - Sort comparators (natural sort, dir-first, case rules).
   - Filter/mask matcher (`*.go`, `!*.tmp`, multi-mask).
   - Human size/date formatters.
   - Selection model (insert/invert/wildcards).
   - Search engine + panelize.
   - Sync comparison logic (pure functions).
   - Keymap validation + defaults.
2. **Model tests** (Bubble Tea): call `Update(msg)` with synthetic messages, assert
   state transitions — no terminal needed. Cover: F5 copy flow, F8 delete confirm,
   dialog open/close, selection, panel swap, resize.
3. **Backend tests**: in-memory fake backend for operation engine; real `os` backend
   against `t.TempDir()` for copy/move/delete/mkdir incl. edge cases (collisions,
   non-empty dirs, symlinks, perms).
4. **Integration/CLI**: `go test ./...`; `make test`; race detector `-race`.
5. **Manual QA checklist** (documented in repo `docs/QA.md`): full F-key tour,
   selection flows, dialog keyboard navigation, resize, CJK filenames, large dirs
   (50k files), hidden files, drive switching, viewer hex search, editor save.
6. **Fuzz**: filter matcher and natural-sort comparator (fuzz on arbitrary strings).

---

## 21. Performance Targets

- Startup to interactive UI: **< 100 ms** (cold start on modern hardware).
- Directory listing of 10,000 entries: < 150 ms; rendering a frame < 16 ms.
- Reading directory metadata lazily (size only on demand for brief/full columns where
  feasible; use `readdirnames` + stat-batching).
- No UI stall during a 10 GB copy (async engine + buffered progress throttle, e.g.
  progress msg every 100 ms max).
- Memory: flat directory lists (no per-frame allocations for column headers, etc.).

---

## 22. Milestones / Roadmap

### Milestone 1 — Skeleton & Panels (MVP)
- Bubble Tea bootstrap, alt-screen, window-size handling, quit (`F10`/`Ctrl+C`).
- Two panels, brief + full display, cursor navigation, enter/.., parent, re-read.
- Command line echo + execution via shell; panel tracks typed `cd`.
- Function-key bar rendering.
- Config load/save (layout + options).

**Definition of done:** can navigate the filesystem, run commands, persist layout.

### Milestone 2 — File Operations
- Copy/move/mkdir/delete with dialogs, selection (Insert/+/−/*), progress, cancel.
- chmod/chown/attrs, size calc.
- Confirmation dialogs, overwrite policies.

### Milestone 3 — Viewer & Editor
- Text + hex viewer with search and quick-view-in-panel.
- Editor: cursor/insert/delete/save/search (v1 scope).

### Milestone 4 — Menus, Search, Sync
- Full top menu, tree view, quick view, drive switching, panel swap.
- Find files + panelize; directory sync; compare.
- User menu & extension-file editing.

### Milestone 5 — Polish & Extensions
- Themes/colors dialog, mouse support, trash, command history UI, bookmarks.
- Stretch: archive VFS (browse/extract), SFTP backend, tabs.

---

## 23. Acceptance Criteria (v1)

1. `go build` produces a single static binary; runs on macOS, Linux, Windows.
2. Screen matches the NC layout in §5; F1–F10 behave as §7.
3. Copy/move/delete/mkdir work with and without multi-selection; progress shown;
   Esc cancels; no UI freeze (measured).
4. Viewer (text+hex) and editor functional per §11.
5. Command line executes shell commands; `cd`/drive changes update active panel.
6. Configuration file round-trips; keymap and layout changes persist.
7. Find-files and sync operate correctly on a 5,000-file test tree.
8. `make lint test` clean; race-detector tests pass.

---

## 24. Security Considerations

- Never exec user input without going through the intended shell path (documented
  behavior — NC is a shell wrapper, this is by design; do not add injection filters
  that break shell semantics).
- Path traversal: operations use resolved absolute paths; reject `..` escapes in
  archive VFS entry names (zip-slip protection).
- Config files treated as trusted local input; no network.
- Symlink handling in copy: default copy-as-link to avoid traversal surprises; UI
  warning when copying a symlink target outside source tree (v2).

---

## 25. References

- Wikipedia — Norton Commander (history, versions, features).
- Softpanorama — *The History of Development of Norton Commander* (NC 1.0–5.51
  feature timeline; archive VFS, search VFS, sync, user menu).
- streetinfo.lu — *DOS software: Norton Commander* (operation dialogs, selection keys,
  viewer/editor, extension-file editing workflow).
- WinNc keyboard shortcuts reference (NC key vocabulary incl. `Ctrl+R`, `Ctrl+E`,
  `Ctrl+F`, `Ctrl+S`, `Ctrl+B`, numpad `+ - *`).
- en.pjw48.net — *Norton Commander: A Deep Dive* (feature summary).
- Charmbracelet — Bubble Tea v2 / Lip Gloss v2 / Bubbles (framework docs).
- Existing Go dual-pane file managers for reference patterns:
  - `kooler/middaycommander` (dual-panel, archive browse, config.toml keymap).
  - `feherk/vc` (Volkov Commander clone; tview + tcell; F-key map, DOS blue theme).
  - `mogglemoss/pelorus` (Bubble Tea + Lip Gloss stack, dual-pane, async op queue).
  - `mrpbennett/bucky` (Charm stack, backend interface, two-phase transfers).

---

## Appendix A — Default Keymap (complete)

| Chords | Action |
|---|---|
| `F1` | `help` |
| `F2` | `user_menu` |
| `F3` | `view` |
| `F4` | `edit` |
| `F5` | `copy` |
| `F6` | `move` |
| `F7` | `mkdir` |
| `F8` | `delete` |
| `F9` | `menu` |
| `F10` | `quit` |
| `F11` | `attributes` |
| `Tab` | `switch_panel` |
| `Enter` | `open` |
| `Backspace` | `parent` |
| `Insert` | `toggle_select` |
| `Ctrl+F1` / `Ctrl+F2` | `toggle_left` / `toggle_right` |
| `Ctrl+F5` | `copy_same_dir` |
| `Ctrl+F8` | `sync_dirs` |
| `Ctrl+F11` | `panel_filter` |
| `Ctrl+R` | `reread` |
| `Ctrl+U` | `swap_panels` |
| `Ctrl+L` | `system_info` |
| `Ctrl+O` | `terminal` |
| `Ctrl+P` | `panelize` |
| `Ctrl+E` | `recall_command` |
| `Ctrl+Enter` | `insert_filename` |
| `Ctrl+\` | `goto_root` |
| `Alt+F1` / `Alt+F2` | `change_drive_left` / `change_drive_right` |
| `Alt+F7` | `find` |
| `Alt+F9` | `size_calc` |
| `Alt+F10` | `tree` |
| Numpad `+` / `-` / `*` | `select_mask` / `unselect_mask` / `invert_selection` |
| `Space` | `calc_size` |

## Appendix B — Open Design Questions

1. Default editor: full internal nc-style editor vs delegate to `$EDITOR`?
   (Recommendation: internal minimal editor v1, `$EDITOR` option.)
2. Trash on by default or hard delete with confirm? (Recommendation: trash ON.)
3. Tabs-per-panel in v1 or v2? (Recommendation: v2.)
4. Show menu bar always or only on F9? (Recommendation: configurable, default hidden.)
5. Truecolor 24-bit default or classic 16-color ANSI? (Recommendation: 16-color
   default for fidelity, truecolor theme option.)
