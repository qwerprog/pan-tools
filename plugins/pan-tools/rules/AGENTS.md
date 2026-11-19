# pan-tools Plugin Safety & Execution Rules

When interacting with Baidu Netdisk through `pant` (`pan-tools`):

1. **Always use non-interactive machine-friendly flags**:
   - Pass `-q --no-color` on every `pant` invocation, and `--json` (or `--plain`) on commands that support structured output (`ls`, `search`, `who`, `loglist`, `quota`, `share list`, `recycle list`, `offlinedl list`).
   - Never start the interactive `pant` REPL (bare `pant` with no subcommand) or the fullscreen TUI (`pant ui` / `tui` / `browse`) inside an automated shell command.

2. **Confirm before destructive or public-exposure actions**:
   - Require explicit user confirmation before executing `pant rm`, `pant recycle delete` (especially `-all`), `pant share set`, `pant logout`, or `pant config reset`.
   - Always show the exact target remote paths or IDs before asking for confirmation.

3. **Protect account credentials**:
   - Never print, log, summarize, or commit `BDUSS`, `STOKEN`, `PTOKEN`, raw `Cookies`, or the contents of `~/.config/pan-tools/pcs_config.json`.
   - If the user is not logged in, instruct them to run `pant login -qr` in their own terminal rather than asking them to paste credentials into chat unless they explicitly choose to do so.
