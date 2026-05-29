# NixPeek

NixPeek is a cross-platform TUI for searching Nix packages with live suggestions and an attrPath-first workflow.

## Overview

NixPeek is designed for fast package discovery while keeping declarative configuration workflows in mind.

Core capabilities:

- Live search (`nix search nixpkgs <query> --json`) while typing
- attrPath-first results for copy/paste into Nix configs
- Installed state marker (`installed` / `not installed`)
- Search filters for scope, match mode, and exact attrPath
- Split view with package details panel
- Quick actions for:
  - attrPath copy
  - `nix profile install` command copy
  - `nix run` command copy
- Session cache for faster repeated queries
- Backend interface ready for future web-index support

## Requirements

- Nix CLI available in `PATH` (for `nix search` and installed checks)
- Go 1.22+ (for local build)

## Install / Build

Install with Nix flakes from GitHub:

```bash
nix profile install github:voidhartt/NixPeek
```

If flakes are not enabled globally:

```bash
nix --extra-experimental-features "nix-command flakes" profile install github:voidhartt/NixPeek
```

Run directly without installing:

```bash
nix run github:voidhartt/NixPeek
```

Pin to a specific revision:

```bash
nix profile install github:voidhartt/NixPeek?rev=c8f123b22b4277120b25ac8b460008b9a6928d5e
```

Build from the local flake:

```bash
nix build .#nixpeek
```

Build from source:

```bash
go mod tidy
go build -o nixpeek ./cmd/nixpeek
```

Optional quick run without creating a binary:

```bash
go run ./cmd/nixpeek
```

## Usage

Run with defaults:

```bash
nixpeek
```

Start with a pre-filled query:

```bash
nixpeek --query "ripgrep"
```

Select backend explicitly:

```bash
nixpeek --backend local
```

## Keybindings

- `q` / `ctrl+c`: quit
- `?`: help overlay
- `tab`: toggle details panel
- `enter`: focus search input
- `esc`: return to list mode
- `up/down` or `j/k`: move selection
- `ctrl+n`: search scope `name`
- `ctrl+d`: search scope `name + description`
- `ctrl+p`: match mode `prefix`
- `ctrl+o`: match mode `contains`
- `ctrl+e`: exact attrPath toggle
- `alt+c`: copy attrPath
- `alt+i`: copy `nix profile install` command
- `alt+r`: copy `nix run` command
- `c` / `i` / `r`: same actions from list mode

## Development

Run tests:

```bash
go test ./...
```

Build validation:

```bash
go build ./cmd/nixpeek
```
