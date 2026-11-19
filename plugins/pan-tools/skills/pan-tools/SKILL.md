---
name: pan-tools
description: >-
  Browse, search, download, upload, transfer share links, manage offline downloads,
  shares, recycle bin, and multi-account quotas on Baidu Netdisk (百度网盘) using
  the `pant` Go CLI. Use when the user asks to inspect Baidu Netdisk folders or
  quota, search cloud files, download/upload files, save shared links/codes,
  manage recycle bin or offline download tasks, or switch Baidu Netdisk accounts.
  Do not use for Google Drive, Dropbox, or local-only filesystem tasks.
metadata:
  icon: ☁️
---

# pan-tools (`pant`)

Operate Baidu Netdisk from the command line using the `pant` Go CLI with structured `--json` / `--plain` output.

## 1. Locate `pant` Binary

Prefer `pant` on `PATH`, or invoke the bundled resolver script [scripts/pant.sh](./scripts/pant.sh) (Linux/macOS) / [scripts/pant.cmd](./scripts/pant.cmd) (Windows), which resolves `$PANT_BIN`, `PATH`, `GOPATH/bin`, `~/.local/bin`, cached binaries, or auto-downloads the latest GitHub Release from `qwerprog/pan-tools`:

```bash
<SKILL_DIR>/scripts/pant.sh --version
```

```powershell
# Windows
& "<SKILL_DIR>\scripts\pant.cmd" --version
```

If automatic resolution still fails, ask the user to install `pant` into `PATH` (e.g., `go build -o ~/.local/bin/pant .` from `https://github.com/qwerprog/pan-tools` or download a release binary).

## 2. Check Login State First

Before any Netdisk operation, verify the active account without exposing credentials:

```bash
<SKILL_DIR>/scripts/pant.sh -q --no-color who --json
```

- If an active account is returned, proceed.
- If no account is logged in or authentication fails, **do not** run interactive `pant login` inside a non-TTY shell and **do not** inspect `~/.config/pan-tools/pcs_config.json`. Ask the user to run `pant login -qr` in their own terminal to scan the QR code with the Baidu Netdisk / Baidu App, then retry once they confirm.

## 3. Core Workflows

Always pass `-q --no-color` so banners, ANSI colors, and progress bars do not pollute output.

### Inspect & Search (Read-Only)

```bash
# Account & quota
<SKILL_DIR>/scripts/pant.sh -q --no-color who --json
<SKILL_DIR>/scripts/pant.sh -q --no-color loglist --json
<SKILL_DIR>/scripts/pant.sh -q --no-color quota --json

# List directory & recursive search
<SKILL_DIR>/scripts/pant.sh -q --no-color ls --json /
<SKILL_DIR>/scripts/pant.sh -q --no-color search -r --path / --json "keyword"

# Metadata & directory tree
<SKILL_DIR>/scripts/pant.sh -q --no-color meta /path/to/file
<SKILL_DIR>/scripts/pant.sh -q --no-color tree /path/to/dir
```

### Download & Upload

```bash
# Download to a specific local directory (use --ow only when user approves overwrite)
<SKILL_DIR>/scripts/pant.sh -q --no-color download --saveto /local/dir /remote/file

# Upload local file or directory to a remote folder (--policy: skip | overwrite | rsync)
<SKILL_DIR>/scripts/pant.sh -q --no-color upload --policy skip /local/file /remote/dir
```

> Note: Do not override `-p` (download concurrency) above `1` for non-SVIP accounts, as Baidu may rate-limit the account.

### Save Share Links (转存)

`transfer` (`tf`) saves into the current remote working directory (`pwd`). Switch directory with `cd` first if the user specified a target folder:

```bash
<SKILL_DIR>/scripts/pant.sh -q --no-color cd /target/dir
<SKILL_DIR>/scripts/pant.sh -q --no-color transfer "https://pan.baidu.com/s/..." "提取码"
# Also accepts full Chinese share text containing both URL and extraction code
```

### Shares, Offline Downloads & Recycle Bin

```bash
<SKILL_DIR>/scripts/pant.sh -q --no-color share list --json
<SKILL_DIR>/scripts/pant.sh -q --no-color offlinedl list --json
<SKILL_DIR>/scripts/pant.sh -q --no-color recycle list --json
```

## 4. Safety & Confirmation Boundaries

- **Never launch interactive modes**: Do not run bare `pant`, `pant ui`, `pant tui`, `pant browse`, or bare `pant login`.
- **Confirm before destructive / public actions**: Always show the exact remote paths/IDs and obtain explicit user confirmation before running:
  - `pant rm <paths...>`
  - `pant recycle delete <fs_ids...>` or `pant recycle delete -all`
  - `pant share set <paths...>` (creates a share link)
  - `pant logout` or `pant config reset`
- **Verify results**: Check command exit status and stderr; after write operations (`mkdir`, `mv`, `rename`, `upload`, `transfer`), verify with `ls --json` or `meta` when accuracy matters.

## 5. Full Command Reference

For complete flags and subcommands (`download`, `upload`, `transfer`, `share`, `offlinedl`, `recycle`, `config`, `tool`), read [references/commands.md](./references/commands.md).
