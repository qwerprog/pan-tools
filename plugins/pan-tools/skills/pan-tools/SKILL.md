---
name: pan-tools
description: >-
  Browse, search, download, upload, transfer share links/codes, manage offline
  downloads, shares, recycle bin, and multi-account quotas on Baidu Netdisk
  (百度网盘) via the `pant` Go CLI. Use when the user asks to inspect Baidu
  Netdisk folders, metadata, or quota, search cloud files, download/upload
  files, extract direct links, save shared links/codes, manage recycle bin or
  offline download tasks, encrypt/decrypt files, or switch Baidu Netdisk
  accounts. Do not use for Google Drive, Dropbox, Bilibili, or local-only
  filesystem tasks.
metadata:
  icon: ☁️
---

# pan-tools (`pant`)

Operate Baidu Netdisk from the command line safely and deterministically using the `pant` Go CLI.

## Pre-Flight Check

1. Locate `pant` on `PATH` (`command -v pant`), or invoke the bundled cross-platform resolver script `<SKILL_DIR>/scripts/pant.sh` (Linux/macOS) / `<SKILL_DIR>\scripts\pant.cmd` (Windows) — it checks `$PANT_BIN`, `PATH`, `GOPATH/bin`, `~/.local/bin`, cached binaries, and auto-downloads the latest `qwerprog/pan-tools` GitHub Release if needed:
   ```bash
   <SKILL_DIR>/scripts/pant.sh --version
   ```
   ```powershell
   # Windows
   & "<SKILL_DIR>\scripts\pant.cmd" --version
   ```
   Official user install methods (if manual installation is preferred):
   - Linux / macOS: `curl -fsSL https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.sh | bash` (or `brew install qwerprog/pan-tools/pant`, `yay -S pan-tools-bin`)
   - Windows: `irm https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.ps1 | iex` (or `winget install QwerProg.pan-tools`)
2. Discover exact flags if unsure:
   ```bash
   pant --help
   pant <subcommand> --help
   ```
3. Check authentication state before any Netdisk operation:
   ```bash
   <SKILL_DIR>/scripts/pant.sh -q --no-color who --json
   ```
   - `who --json` exits `0` even when logged out (`{"age":0,"name":"","sex":"","uid":0,"workdir":""}`). Confirm `uid != 0` and `name != ""` before proceeding.
   - If not logged in, **do not** run bare `pant login` in a non-TTY shell and **do not** inspect `~/.config/pan-tools/pcs_config.json`. Instruct the user to run `pant login -qr` in their own terminal and scan the QR code with the Baidu Netdisk or Baidu App, then retry once confirmed.

## Flag Ordering Rule (`urfave/cli` v1)

- Global flags (`-q --no-color`, `--ascii`, `--verbose`) MUST be placed **before** the subcommand.
- Subcommand flags (`--json`, `--plain`, `-r`, `--path`, `--saveto`, `--policy`, `-p`, `-period`, `--collect`, `--download`, `--fs_id`) MUST be placed **after** the subcommand and **before** positional arguments (for example: `pant -q --no-color ls --json /`, NOT `pant -q --no-color ls / --json`).

## Common Workflows

Always pass `-q --no-color` so banners, ANSI colors, and progress bars never pollute stdout.

### 1. Account, Multi-Account & Quota
```bash
pant -q --no-color who --json
pant -q --no-color loglist --json
pant -q --no-color quota --json
pant -q --no-color su <uid_or_name>
```

### 2. Browse, Tree, Search & Metadata
```bash
pant -q --no-color ls --json /
pant -q --no-color ls --plain /工作空间
pant -q --no-color search -r --path / --json "keyword"
pant -q --no-color tree --depth 2 --fsid /dir
pant -q --no-color meta /path/to/file
pant -q --no-color match "/dir/*.pdf"
```

### 3. Directory & File Operations
```bash
pant -q --no-color cd /target/dir
pant -q --no-color pwd
pant -q --no-color mkdir /target/dir/new_folder
pant -q --no-color cp /src/file /dst/dir
pant -q --no-color mv /src/file /dst/dir
pant -q --no-color rename /dir/old.txt new.txt
# Confirm exact paths with user before running rm:
pant -q --no-color rm /path/to/delete
```

### 4. Download & Direct Link
```bash
# Use --ow only when user explicitly approves overwriting local files
pant -q --no-color download --saveto /local/dir --mtime /remote/file
pant -q --no-color download --mode locate --saveto /local/dir /remote/file
pant -q --no-color locate -pan /remote/file
```
> Do not raise `-p` or `-l` above `1` on non-SVIP accounts unless requested.

### 5. Upload (Rapid-Upload & up to 128 GB Chunked Resume)
```bash
# --policy: skip | overwrite | rsync (confirm first before using overwrite)
pant -q --no-color upload --policy skip /local/file /remote/dir
```

### 6. Transfer Share Links or Chinese Share Text (转存)
`transfer` (`tf`) saves into the current remote working directory (`pwd`):
```bash
pant -q --no-color cd /target/dir
pant -q --no-color transfer "https://pan.baidu.com/s/..." "提取码"
# Supports full Chinese share text, --collect, --download, and --fs_id <id>:
pant -q --no-color transfer --collect --download "通过百度网盘分享的文件：..."
```

### 7. Shares, Offline Downloads & Recycle Bin
```bash
# Shares
pant -q --no-color share list --json
# Confirm paths/password/period with user before share set or share cancel:
pant -q --no-color share set -p 8888 -period 7 /remote/path
pant -q --no-color share cancel <share_id>

# Offline downloads (HTTP / FTP / ed2k / Magnet)
pant -q --no-color offlinedl list --json
pant -q --no-color offlinedl add -path=/remote/dir "magnet:?xt=urn:btih:..."
pant -q --no-color offlinedl query <task_id>

# Recycle bin
pant -q --no-color recycle list --json
pant -q --no-color recycle restore <fs_id>
# Confirm with user before deleting permanently:
pant -q --no-color recycle delete <fs_id>
```

### 8. Config, Local Crypto Toolbox & Self-Update
```bash
pant -q --no-color config
pant -q --no-color config set -max_download_rate 20MB/s -proxy socks5://127.0.0.1:1080
pant -q --no-color env
pant -q --no-color tool enc -method aes-256-ctr -key "SecretKey" ./secret.db
pant -q --no-color tool dec -method aes-256-ctr -key "SecretKey" ./secret.db.encrypt
pant -q --no-color tool getip
pant -q --no-color tool showtime
pant -q --no-color sumfile ./archive.tar.gz
pant -q --no-color update
```

## Detailed Reference

For complete command trees, flags, config keys, `PANT_*` variables, and exact `--json` / `--plain` TSV schemas, read [references/commands.md](./references/commands.md).
