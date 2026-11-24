# `pant` Command & Option Reference

Place global options **before** the subcommand, and subcommand flags **after** the subcommand and **before** positional arguments (`urfave/cli` v1 stops flag parsing at the first positional argument).

## Global Flags

| Flag | Description |
| :--- | :--- |
| `-q, --quiet` | Suppress banners, progress bars, and decorative output |
| `--no-color` | Disable ANSI colors (also honors `NO_COLOR=1`) |
| `--ascii` | Force pure 7-bit ASCII borders and symbols (also `PANT_ASCII=1`) |
| `--verbose` | Enable verbose debug logs (also `PANT_VERBOSE=1`) |
| `--prompt-style <style>` | REPL prompt style: `starship` or `boxed` (not needed for one-shot commands) |

## Modules Overview

| Category | Command (Aliases) | Summary / Subcommands |
| :--- | :--- | :--- |
| **Account** | `login` | Authenticate interactively (`-qr`, `-cookies="..."`, `-bduss=... -stoken=...`) |
| | `who` | Show active account (`--json` / `--plain`) |
| | `loglist` | List logged-in accounts (`--json` / `--plain`) |
| | `su` | Switch active account (`pant su <uid_or_name>`) |
| | `logout` | Sign out active account (`-y`; **confirm first**) |
| | `quota` | Query cloud storage quota (`--json` / `--plain`) |
| **Filesystem** | `ui` (`tui`, `browse`) | Fullscreen interactive TUI explorer (never run in agent shells) |
| | `ls` (`l`, `ll`) | List directory (`[-l] [--asc\|--desc] [--time\|--name\|--size] --json\|--plain [dir]`) |
| | `search` (`s`) | Search filenames (`[-r] [-l] [--path=<dir>] --json\|--plain <keyword>`) |
| | `tree` (`t`) | Directory tree (`[--depth <n>] [--fsid] [dir]`) |
| | `cd`, `pwd`, `meta` | Change working dir (`cd [-l]`), print working dir, show metadata (`meta [paths...]`) |
| | `match` | Test remote wildcard matching (`pant match <pattern>`) |
| | `mkdir`, `cp`, `mv`, `rename` (`rn`), `rm` | Create dir, copy, move, rename, move to recycle bin (**confirm `rm` first**) |
| **Transfer** | `download` (`d`) | Multi-thread download (`--saveto`, `--mode locate\|pcs\|stream`, `--mtime`) |
| | `upload` (`u`) | Rapid-upload & chunked upload (`--policy skip\|overwrite\|rsync`) |
| | `locate` (`lt`) | Print direct download URLs (`[-pan] <files...>`) |
| **Cloud** | `transfer` (`tf`) | Save shared links/text to current `pwd` (`[--collect] [--download] [--fs_id <id>]`) |
| | `share` (`sh`) | `set` (**confirm first**), `list` (`[--page <n>] --json\|--plain`), `cancel` (**confirm first**) |
| | `offlinedl` (`od`, `clouddl`) | `add -path=<dir>`, `query`, `list` (`--json\|--plain`), `cancel`, `delete [-all]` |
| | `recycle` (`rc`) | `list` (`[--page <n>] --json\|--plain`), `restore <fs_ids...>`, `delete [-all]` (**confirm first**) |
| **System & Crypto** | `config` | Print config table, `config set <flags...>`, `config reset` (**confirm first**) |
| | `env` | Print `PANT_*` & color environment variables |
| | `tool` | `enc`, `dec`, `getip`, `showtime` (plus aliases to `sumfile` / `match`) |
| | `sumfile` (`sf`) | Compute local file size, MD5, 256 KB slice MD5, CRC32 |
| | `run` | Execute an external OS command |
| | `update` | Check & apply self-update from GitHub Releases (`[-y]`) |

## Key Subcommand Options

### Browse & Search (`ls`, `search`, `tree`)
- `ls` (`l`, `ll`): `-l` (detailed columns), `--asc` / `--desc`, `--time` / `--name` / `--size`, `--json` / `--plain`. (`ll` implies `-l`.)
- `search` (`s`): `-r` (recursive), `-l` (detailed), `--path=<dir>` (default `.`), `--json` / `--plain`.
- `tree` (`t`): `--depth <n>` (`-1` unlimited), `--fsid` (show `fs_id`).

### Download (`download`, `d`) & Direct Link (`locate`, `lt`)
- `--saveto <dir>` / `--save`: Save directly to `<dir>` or current working directory.
- `--ow`: Overwrite existing local files (**confirm first**).
- `--fullpath`: Recreate full remote directory hierarchy under local destination.
- `--mode <locate|pcs|stream>`: Transfer backend (default `locate`).
- `-p <n>`, `-l <n>`, `--retry <n>`: Per-file threads, concurrent files, max retries (default `3`). Keep `-p`/`-l` at `1` unless SVIP.
- `--nocheck`, `--mtime`, `--dindex <n>`, `-x` (chmod `+x` on Unix), `--status`, `--test` (dry run).
- `locate` (`lt`): `-pan` fetches links via Baidu Netdisk home interface.

### Upload (`upload`, `u`)
- `pant -q --no-color upload [--policy skip|overwrite|rsync] [-p <n>] [-l <n>] [--norapid] <local_src...> <remote_dir>`
- Confirm before passing `--policy overwrite`. Supports automatic rapid-upload and up to 128 GB chunked resume.

### Share, Transfer, Offline Download & Recycle Bin
- `transfer` (`tf`): `pant -q --no-color transfer [--collect] [--download] [--fs_id <id>] <url_or_chinese_share_text> [pwd]`
- `share set` (**confirm first**): `-p <passwd>`, `-period <days>` (`0` = permanent), `-f` (combine multi-files).
- `share list`: `--page <n>`, `--json`, `--plain`.
- `share cancel` (**confirm first**): `share cancel <share_ids...>`.
- `offlinedl` (`od`, `clouddl`): `add -path=<dir> <urls...>`, `query <task_ids...>`, `list --json|--plain`, `cancel <task_ids...>` (**confirm first**), `delete [-all] [task_ids...]` (**confirm first**).
- `recycle` (`rc`): `list [--page <n>] --json|--plain`, `restore <fs_ids...>`, `delete [-all] [fs_ids...]` (**confirm first**).

### Config, Crypto Toolbox & Self-Update
- `config set` key flags: `-max_parallel`, `-max_upload_parallel`, `-max_download_load`, `-max_upload_load`, `-max_download_rate` (`20MB/s`), `-max_upload_rate`, `-cache_size` (`128KB`), `-savedir`, `-upload_policy`, `-proxy` (`socks5://...` / `http://...`), `-proxy_hostnames`, `-local_addrs`, `-prompt_style` (`starship` / `boxed`), `-enable_https`, `-no_check`.
- `tool enc` / `tool dec`: `-method <aes-128-ctr|aes-192-ctr|aes-256-ctr|aes-128-cfb|...|aes-256-ofb>` (default `aes-128-ctr`), `-key <secret>`, `--disable-gzip`.
- `tool getip` / `tool showtime` / `sumfile <files...>` / `update [-y]`.

### Environment Variables (`pant env`)
`PANT_CONFIG_DIR`, `PANT_ASCII` (`1`), `PANT_VERBOSE` (`1`), `PANT_PROMPT_STYLE`, `PANT_UPDATE_REPO`, `NO_COLOR`, `CLICOLOR`, `CLICOLOR_FORCE`.

## Structured Output (`--json` & `--plain`) Schemas

Only these **8 commands** support `--json` and `--plain`:

1. **`who --json`**: `{ "uid": uint64, "name": string, "sex": string, "age": float, "workdir": string }`
   - Logged out state still exits `0` with `uid: 0` and `name: ""`.
   - `who --plain`: `<uid>\t<name>\t<sex>\t<age>\t<workdir>`
2. **`loglist --json`**: `[ { "uid", "name", "sex", "age", "workdir" }, ... ]`
   - `loglist --plain`: `<index>\t<uid>\t<name>\t<sex>\t<age>\t<workdir>`
3. **`quota --json`**: `{ "username": string, "quota_bytes": int64, "used_bytes": int64, "free_bytes": int64, "used_percent": float }`
   - `quota --plain`: `<username>\t<quota_bytes>\t<used_bytes>\t<used_percent%>`
4. **`ls --json` / `search --json`**: `[ { "fs_id", "app_id", "path", "name", "size", "is_dir", "ctime", "mtime", "md5" }, ... ]`
   - `--plain` TSV: `<fs_id>\t<file|dir>\t<size>\t<YYYY-MM-DD HH:MM:SS>\t<name_or_path>`
   - **Parsing note**: Always pass `-F'\t'` to `awk` because column 4 (`YYYY-MM-DD HH:MM:SS`) contains a space.
5. **`share list --json`**: `[ { "share_id", "shortlink", "passwd"?, "typical_path", "valid", "view_count" }, ... ]`
   - `share list --plain`: `<share_id>\t<shortlink>\t<passwd>\t<typical_path>\t<valid>\t<view_count>`
6. **`offlinedl list --json`**: JSON array of cloud-download task structs.
   - `offlinedl list --plain`: `<task_id>\t<task_name>\t<file_size>\t<save_path>\t<status_text>\t<source_url>`
7. **`recycle list --json`**: JSON array of recycle-bin entry structs.
   - `recycle list --plain`: `<fs_id>\t<size>\t<left_time>\t<mtime>\t<path>`
