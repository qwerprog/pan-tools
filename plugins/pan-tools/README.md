# pan-tools Plugin for Antigravity

Antigravity AI Agent plugin for operating Baidu Netdisk (百度网盘) via the `pant` Go CLI.

## Included Customizations

- `plugin.json`: Plugin metadata (`5.0.1`) and suggested prompts.
- `assets/logo.svg`: 128×128 cloud-transfer badge aligned with `pan-tools` branding.
- `rules/AGENTS.md`: Mandatory safety guardrails (flag-ordering rules, non-interactive enforcement, credential protection, rate-limit guard, and confirmation before destructive/public operations).
- `skills/pan-tools/SKILL.md`: Progressive-disclosure workflow covering pre-flight checks, login status verification, browsing, searching, transfer links, downloads/uploads, shares, offline downloads, recycle bin, config, and local crypto toolbox.
- `skills/pan-tools/references/commands.md`: Source-verified CLI flag reference and exact `--json` / `--plain` TSV schemas.
- `skills/pan-tools/scripts/pant.sh` & `pant.cmd`: Cross-platform binary resolver (`$PANT_BIN` → `PATH` → `GOPATH/bin` → `~/.local/bin` → cached GitHub Release auto-download).

## Install in Antigravity

### Remote Install (Recommended)

```bash
agy plugin install https://github.com/qwerprog/pan-tools/tree/main/plugins/pan-tools
```

### Local Directory Install

In Antigravity's **Plugins** panel, choose **`+ Install from local directory...`** and point to `plugins/pan-tools`, or run:

```bash
agy plugin install /path/to/pan-tools/plugins/pan-tools
```

## Prerequisites (`pant` CLI)

If `pant` is not yet installed on `PATH`, the plugin's resolver script can auto-cache the latest release binary, or you can install `pant` directly:

- **Linux / macOS**: `curl -fsSL https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.sh | bash` (or `brew install qwerprog/pan-tools/pant`, `yay -S pan-tools-bin`)
- **Windows**: `irm https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.ps1 | iex` (or `winget install QwerProg.pan-tools`)

Then log in once from your own terminal before invoking agent workflows:

```bash
pant login -qr
```
