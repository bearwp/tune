# AGENTS.md

## Project overview

`tune` is a terminal MP3 player TUI built with the [Charm](https://charm.sh) stack
(Bubble Tea, Bubbles, Lip Gloss) and the [beep](https://github.com/faiface/beep)
audio library. It scans directories for `.mp3` files, shows a searchable library,
lets you stage tracks into a play queue, and plays them with seek/volume/shuffle/repeat.

It is one app in a larger suite of Charm TUIs that all depend on a shared local
module `charmkit` (see "Local dependency" below).

## Commands

There are no Makefiles, CI configs, lint configs, or test files in this repo.

```sh
go build              # produces the `tune` binary (gitignored)
go run .              # build and run
go run . /path/to/dir # play the .mp3 files in a directory (or pass file paths)
```

`go vet` and `gofmt` are used implicitly by the editor's LSP; no custom lint tooling
is configured. There are no tests.

## Module / layering

- `go.mod` — module `github.com/bear/tune`, Go `1.26.1`.
- Single `package main`, four files, no subpackages:
  - `main.go` — CLI entry point; file discovery and filename parsing.
  - `model.go` — the Bubble Tea `Model` (all UI state, key handling, layout).
  - `player.go` — `Player`, a thin wrapper around `beep` for MP3 playback.
  - `styles.go` — Lip Gloss styles, sourced from the `charmkit` palette.
- `.gitignore` ignores the compiled `tune` binary (build artifact lives in the repo root).

## Local dependency: `charmkit` (important)

`go.mod` contains:

```
replace github.com/bear/charmkit => ../charmkit
```

`charmkit` is a sibling directory (`../charmkit`) shared across the "Bear" app
suite. It provides:

- `charmkit.Colors` — the semantic ANSI palette (`Accent`, `Muted`, `Subtle`,
  `Text`, etc.). Use it instead of hardcoding colors so the app inherits the
  active Omarchy/pywal terminal theme.
- `charmkit.NewKeys()` — universal keybindings (`Quit`, `Help`, `Back`, `Up`
  (`k`), `Down` (`j`), `Tab`, `Enter`, `Delete`, ...). App-specific bindings are
  layered on top in `model.go`'s `keyMap`.
- `charmkit.Help(width, short, full, showFull)` — `tune` renders its bottom
  bar through this (short help normally, full table when `?` toggles it). It
  width-clamps via `MaxWidth`, so the help line never overflows the terminal.
- `charmkit.ContentWidth(...)` — not used by `tune` yet, but available.

Because this is a `replace` directive, edits to `../charmkit` immediately affect
this app. When changing shared conventions (keys, colors, help styling), update
`charmkit` there, not locally in this repo.

## Architecture / data flow

Control flow:

1. `main()` calls `gatherTracks()` to build a `[]track` from CLI args (defaults
   to `~/Music`, then `~/music`, then `$HOME`).
2. It constructs one `Player` and calls `InitSpeaker()` once (the `beep` sample
   rate / audio device is a process-global singleton).
3. `tea.NewProgram(NewModel(tracks, player), tea.WithAltScreen()).Run()` starts
   the event loop.

UI model:

- Two panes: a `Library` list and a `Playlist` (queue) list. `Tab` toggles
  `Model.focus` between them (`focusLibrary` / `focusQueue`).
- A track is *staged* (space) from the library into the queue; the queue is the
  only thing that is ever played. "Now playing" is always an index into the
  queue (`Model.current`).
- When the terminal is wide enough (`width >= 72`), both panes show side by
  side; otherwise only the focused pane is drawn.
- The header (now-playing + progress bar) and footer are progressively hidden
  as height shrinks — see `resize()`.

Audio:

- `Player` wraps `beep.StreamSeekCloser` + `beep.Ctrl` (pause) + `effects.Volume`.
- Playback advances in a `time.Second/4` `tickMsg` loop; when position reaches
  length, the model auto-advances via `next()` (or re-seeks on repeat).

## Non-obvious conventions / gotchas

- **`track` is a value type, and `bubbles/list` stores copies.** The library and
  queue hold `track` by value (it implements `list.Item`). Mutating a `track`
  (e.g. setting `staged` or `playing`) does *not* update the rendered list — you
  must call `SetItem`/`InsertItem`/`RemoveItem` on the list model afterward.
  `stage`, `unstageAt`, and `syncPlaying` follow this pattern; new code changing
  track state must do the same.
- **`Model.current` indexes into `m.queue`**, never into `m.library`. Removing
  items from the queue (via `removeFromQueue`) carefully re-adjusts `current`
  and re-plays the `wasPlaying` track if it was the one removed.
- **`syncPlaying()`** is the single place that reconciles the `playing` flag
  across library and queue lists; call it after any change to `current` or the
  queue contents.
- **Metadata comes from filenames**, not ID3 tags. `parseName` splits on
  `" - "` into `artist` / `title`. Track duration is read by fully decoding the
  MP3 header in `MP3Duration` (opening each file a second time at startup).
- **`InitSpeaker()` must be called before any playback**, but `main` tolerates
  failure (prints a warning and continues) — the UI still runs with no audio.
- There is no `Ctrl`/`Stop` cleanup on normal exit beyond `player.Stop()` on
  quit; `speaker` is process-global and cannot be re-initialized with different
  settings.
- The `focus` and `wide` layout flags are recomputed in `resize()`, which is
  called from `tea.WindowSizeMsg` and defensively from `View()` when `width == 0`.

## Style

- Tabs for indentation, idiomatic Go formatting (`gofmt`).
- Semantic color use via `styles.go` aliases (`accentStyle`, `mutedStyle`,
  `titleStyle`, `artistStyle`, `subtleStyle`, `box`) — do not inline hex/ANSI
  color literals in UI code.
- Comments document *why* (layout collapse order, value-vs-pointer list
  semantics), not what.