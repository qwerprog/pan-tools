# pan-tools Plugin for Antigravity

Bundle that connects Antigravity to Baidu Netdisk (百度网盘) via the `pant` Go CLI.

## Included Customizations

- `plugin.json`: Plugin metadata and suggested prompts.
- `skills/pan-tools/SKILL.md`: Progressive-disclosure agent workflow for browsing, searching, uploading, downloading, and transferring Baidu Netdisk files with `-q --no-color --json`.
- `skills/pan-tools/scripts/pant.sh`: Cross-path `pant` binary resolver (`$PANT_BIN` → `PATH` → `GOPATH/bin` → `~/.local/bin` → repo `./pant`).
- `skills/pan-tools/references/commands.md`: Complete verified CLI flag reference.
- `rules/AGENTS.md`: Safety guardrails (non-interactive enforcement, credential protection, confirmation before destructive/public operations).

## Install Locally in Antigravity

1. Build or install `pant` (recommended on `PATH`, e.g., `go build -o ~/.local/bin/pant .`).
2. Log in once in your terminal:
   ```bash
   pant login -qr
   ```
3. In Antigravity's **Plugins** panel, choose **`+ Install from local directory...`** and enter the absolute path to this folder (`plugins/pan-tools`).
