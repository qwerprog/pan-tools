# pan-tools (`pant`) CLI Reference

## Global Options

Place global options before the subcommand:

- `-q`, `--quiet`: Suppress banners, progress bars, and decorative output.
- `--no-color`: Disable ANSI colors (also honors `NO_COLOR=1`).
- `--ascii`: Force pure 7-bit ASCII borders/symbols (also `PANT_ASCII=1`).
- `--verbose`: Enable verbose debug logs (also `PANT_VERBOSE=1`).
- `--prompt-style <starship|boxed>`: REPL prompt style (not needed for single commands).

## Commands Supporting `--json` and `--plain`

- `pant -q --no-color who --json`
- `pant -q --no-color loglist --json`
- `pant -q --no-color quota --json`
- `pant -q --no-color ls [--asc|--desc] [--time|--name|--size] [-l] --json [path]`
- `pant -q --no-color search [-r] [-l] [--path <dir>] --json <keyword>`
- `pant -q --no-color share list [--page <n>] --json`
- `pant -q --no-color offlinedl list --json`
- `pant -q --no-color recycle list [--page <n>] --json`

## Filesystem Operations

- `cd [-l] <dir>` — Change remote working directory.
- `pwd` — Print current remote working directory.
- `tree <dir>` — Print directory tree.
- `meta <paths...>` — Show metadata (size, md5, fs_id, timestamps).
- `mkdir <dir>` — Create remote directory.
- `cp <src...> <dst>` — Copy remote files/directories.
- `mv <src...> <dst>` — Move or rename remote files/directories.
- `rename <src> <new_name>` (`rn`) — Rename a single file/directory.
- `rm <paths...>` — **Destructive**: Move files/directories to recycle bin (confirm first).
- `match <pattern>` — Test shell wildcard matching against remote paths.

## Transfer Operations

### `download` (`d`)
- `--saveto <dir>`: Save to target local directory.
- `--save`: Save directly into current working directory.
- `--ow`: Overwrite existing local files.
- `--fullpath`: Preserve full remote directory hierarchy locally.
- `--mode <locate|pcs|stream>`: Download mode (default `locate`).
- `-p <n>`: Concurrency threads (`0` uses configured default).
- `-l <n>`: Max concurrent files (`0` uses configured default).
- `--retry <n>`: Max retries (default `3`).
- `--nocheck`: Skip MD5 verification after download.
- `--mtime`: Preserve server modification time locally.
- `--test`: Dry-run download without saving to disk.

### `upload` (`u`)
- `--policy <skip|overwrite|rsync>`: Conflict handling policy (default `skip`).
- `--norapid`: Skip rapid-upload hash matching.
- `-p <n>`, `-l <n>`, `--retry <n>`: Parallelism and retry controls.

### `transfer` (`tf`)
- `pant -q --no-color transfer [--collect] [--download] [--fs_id <id>] <share_url_or_text> [pwd]`
- Saves into the active remote working directory (`cd` first if needed).

### `locate` (`lt`)
- `pant -q --no-color locate <paths...>` — Print direct download URLs.

## Cloud Features

- **Share (`share`, `sh`)**: `share list --json`, `share set <paths...>` (**confirm first**), `share cancel <share_ids...>`.
- **Offline Download (`offlinedl`, `od`)**: `offlinedl list --json`, `offlinedl add -path=<dir> <urls...>`, `offlinedl query <task_ids...>`, `offlinedl cancel <task_ids...>`, `offlinedl delete <task_ids...>`.
- **Recycle Bin (`recycle`, `rc`)**: `recycle list --json`, `recycle restore <fs_ids...>`, `recycle delete <fs_ids...>` / `recycle delete -all` (**confirm first**).

## Account & System

- `login -qr` / `login -cookies="..."` / `login -bduss=... -stoken=...`
- `su <uid_or_name>` — Switch active Baidu account.
- `logout` — Log out active account (**confirm first**).
- `config`, `config set ...`, `config reset` (**confirm first**).
- `env` — Show `PANT_*` and color environment variables.
- `tool enc|dec|sumfile|match|getip|showtime` — Built-in utility toolbox.
- `update` — Check releases from `PANT_UPDATE_REPO` (default `qwerprog/pan-tools`).
