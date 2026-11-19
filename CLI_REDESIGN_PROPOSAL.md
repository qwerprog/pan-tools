# Comprehensive CLI & TUI Redesign Proposal for pan-tools

**Document Version**: 1.1.0  
**Target Repository**: `github.com/qjfoidnh/BaiduPCS-Go` (pan-tools `pant` v4.0.2-dev)  
**Deliverable**: Architectural Specification & UI/UX Redesign Documentation  
**License Compliance**: Apache-2.0 Open-Source Fork & Rebranding Specification  

---

## Implementation Status Matrix (Current Codebase Alignment)

> Note: Below summarizes which parts of this design proposal have been landed in the repository versus items intentionally deferred to future releases.

| Proposal Area | Selected Option / Design | Current Status | Codebase Location |
| :--- | :--- | :--- | :--- |
| **Brand & Binary Name** | **pan-tools** (`pant`), using `~/.config/pan-tools` (`%APPDATA%\pan-tools` on Windows) and `PANT_CONFIG_DIR` | **Implemented** | `main.go`, `internal/cliapp/`, `internal/pcsconfig/pcsconfig.go`, `.github/workflows/main.yml` |
| **Color Palette & Theme** | **Atom One Dark** semantic tokens (`#61AFEF` Blue, `#56B6C2` Cyan, `#98C379` Green, `#E5C07B` Amber, `#E06C75` Coral) + `NO_COLOR` / `CLICOLOR` / `CLICOLOR_FORCE` / `--no-color` / `--ascii` | **Implemented** | `internal/ui/theme.go`, `internal/ui/badge.go` |
| **Startup Banner & Logo** | pan-tools geometric vault monogram (`╭──◈──╮`) + responsive layouts (`>=112` side-by-side, `74-111` stacked `<=80` cols, `<74` compact, non-TTY/`--quiet` suppressed) + UID partial masking (`MaskUID`) | **Implemented** | `internal/ui/logo.go`, `internal/ui/banner.go`, `internal/cliapp/repl.go` |
| **Two-Line REPL Prompt** | Configurable two-line REPL prompt styles (`starship` right-aligned default: `alice  SVIP 10  ~/工作/架构设计    54.2% · 1.1TB/2.0TB` + `❯ `; `boxed` rounded frame: `╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]` + `╰─❯ `) via `prompt_style` / `PANT_PROMPT_STYLE` / `--prompt-style`, avoiding `liner.ErrInvalidPrompt` | **Implemented** | `internal/ui/prompt.go`, `baidupcs/membership.go`, `internal/pcsconfig/`, `internal/cliapp/` |
| **Tables, Dashboard & Machine Output** | Lipgloss rounded Unicode / pure-ASCII tables (`pcstable` -> `internal/ui`), semantic file coloring (`filetype.go`), multi-task `TransferDashboard` (`dashboard.go`), per-command `--json` & `--plain` TSV modes, single-point `os.Stderr` error rendering | **Implemented** | `internal/ui/{table,dashboard,filetype,output,notify}.go`, `pcstable/` |
| **Fullscreen Cloud Explorer TUI** | Bubble Tea interactive file manager (`pant ui` / `tui` / `browse`) with split inspector, `/` filter, `Space` multi-select, mkdir/rename/delete & `cd`-on-quit | **Implemented** | `internal/tui/`, `internal/cliapp/cmd_fs.go` |
| **CLI Flag Parser Migration** | Migration from `urfave/cli` v1 to `spf13/cobra` | **Deferred (Optional)** | Currently modularized on `urfave/cli` in `internal/cliapp/` to preserve 100% command/flag compatibility |

---

## Table of Contents

1. [Executive Summary & Problem Statement](#1-executive-summary--problem-statement)
   - [1.1 Context & Project Evolution](#11-context--project-evolution)
   - [1.2 Comprehensive Defect Inventory of Existing CLI & REPL](#12-comprehensive-defect-inventory-of-existing-cli--repl)
   - [1.3 Strategic Objectives of the Redesign](#13-strategic-objectives-of-the-redesign)
2. [Comparative Analysis of Popular Go CLI Design Patterns](#2-comparative-analysis-of-popular-go-cli-design-patterns)
   - [2.1 Benchmark Evaluation Matrix](#21-benchmark-evaluation-matrix)
   - [2.2 Command Grammar & Hierarchy Patterns](#22-command-grammar--hierarchy-patterns)
   - [2.3 Visual Tokens & Design Hierarchy](#23-visual-tokens--design-hierarchy)
   - [2.4 Interactive Shell & REPL Aesthetics](#24-interactive-shell--repl-aesthetics)
   - [2.5 Dual Output Modes (Human vs Machine-Readable Automation)](#25-dual-output-modes-human-vs-machine-readable-automation)
3. [Project Naming Recommendations (Requirement R1)](#3-project-naming-recommendations-requirement-r1)
   - [3.1 Strategic Rebranding Rationale](#31-strategic-rebranding-rationale)
   - [3.2 Project Naming Evaluation Matrix](#32-project-naming-evaluation-matrix)
   - [3.3 Detailed Candidate Profiles](#33-detailed-candidate-profiles)
     - [Candidate 1: CloudPacer (`pacer`)](#candidate-1-cloudpacer-pacer)
     - [Candidate 2: PanFlux (`pflux`)](#candidate-2-panflux-pflux)
     - [Candidate 3: KumoCLI (`kumo`)](#candidate-3-kumocli-kumo)
     - [Candidate 4: DrivePilot (`pilot`)](#candidate-4-drivepilot-pilot)
     - [Candidate 5: AetherPan (`aether`)](#candidate-5-aetherpan-aether)
     - [Candidate 6: PanGo (`pango`)](#candidate-6-pango-pango)
   - [3.4 Final Maintainer Recommendation](#34-final-maintainer-recommendation)
4. [Terminal ASCII Art Logo Designs (Requirement R2)](#4-terminal-ascii-art-logo-designs-requirement-r2)
   - [4.1 Terminal Width Constraint & Mathematics](#41-terminal-width-constraint--mathematics)
   - [4.2 Candidate 1: "CloudVault / CloudPCS" (Modern Cloud & Storage Vault)](#42-candidate-1-cloudvault--cloudpcs-modern-cloud--storage-vault)
   - [4.3 Candidate 2: "VortexTransfer / FastPCS" (High-Speed Turbo Transfer)](#43-candidate-2-vortextransfer--fastpcs-high-speed-turbo-transfer)
   - [4.4 Candidate 3: "PanGo / Modern Geometric" (Charm TUI Minimalist Monogram)](#44-candidate-3-pango--modern-geometric-charm-tui-minimalist-monogram)
   - [4.5 Candidate 4: "PanGo Classic Heritage" (Retro Terminal Block)](#45-candidate-4-pango-classic-heritage-retro-terminal-block)
   - [4.6 Verification of Standard Terminal Width (<= 80 Columns)](#46-verification-of-standard-terminal-width--80-columns)
5. [Modern CLI & Interactive Shell UI/UX Redesign Specification (Requirement R3)](#5-modern-cli--interactive-shell-uiux-redesign-specification-requirement-r3)
   - [5.1 Responsive Startup Banner Specification](#51-responsive-startup-banner-specification)
     - [Mode A: Standard 80-Column Stacked Box Layout (76 Cols Exact)](#mode-a-standard-80-column-stacked-box-layout-76-cols-exact)
     - [Mode B: Wide Terminal Side-by-Side 2-Column Split Layout (111 Cols Exact)](#mode-b-wide-terminal-side-by-side-2-column-split-layout-111-cols-exact)
     - [Mode C: Narrow / Compact View (< 70 Cols)](#mode-c-narrow--compact-view--70-cols)
     - [Mode D: Headless / Script / Pipe Mode](#mode-d-headless--script--pipe-mode)
     - [Mode E: NO_COLOR Compliance & Pure ASCII Fallback](#mode-e-no_color-compliance--pure-ascii-fallback)
   - [5.2 Interactive Shell REPL Prompt Redesign](#52-interactive-shell-repl-prompt-redesign)
     - [Architectural Discovery: Liner's ANSI Restriction](#architectural-discovery-liners-ansi-restriction)
     - [The Starship-Style Two-Line Right-Aligned Prompt Architecture](#the-starship-style-two-line-right-aligned-prompt-architecture)
     - [Contextual Prompt States & Tab Autocompletion](#contextual-prompt-states--tab-autocompletion)
   - [5.3 Status Indicators, Badges & Contextual Error Hierarchy](#53-status-indicators-badges--contextual-error-hierarchy)
   - [5.4 Modern Output & Table Formatting](#54-modern-output--table-formatting)
   - [5.5 Modernized Progress Bar Engine](#55-modernized-progress-bar-engine)
6. [Modern Go TUI/CLI Library Feasibility & Coexistence Architecture](#6-modern-go-tuicli-library-feasibility--coexistence-architecture)
   - [6.1 Library Capability & Compatibility Evaluation Matrix](#61-library-capability--compatibility-evaluation-matrix)
   - [6.2 Decoupled UI Presentation Layer Architecture (`internal/ui`)](#62-decoupled-ui-presentation-layer-architecture-internalui)
   - [6.3 Modal Execution Pattern for Bubble Tea Coexistence](#63-modal-execution-pattern-for-bubble-tea-coexistence)
   - [6.4 Zero-Overhead Lipgloss Declarative Styling](#64-zero-overhead-lipgloss-declarative-styling)
7. [Migration Plan, Backward Compatibility & Implementation Roadmap](#7-migration-plan-backward-compatibility--implementation-roadmap)
   - [7.1 Four-Phase Implementation Roadmap](#71-four-phase-implementation-roadmap)
   - [7.2 Backward Compatibility Guarantees](#72-backward-compatibility-guarantees)
   - [7.3 Verification & Validation Protocols](#73-verification--validation-protocols)

---

## 1. Executive Summary & Problem Statement

### 1.1 Context & Project Evolution

PanGo evolved from earlier community Go clients created by `iikira` and maintained by contributors such as `qjfoidnh`. Written in Go, it has earned a reputation as one of the most capable command-line utilities for interacting with Baidu Netdisk (Baidu Pan, 百度网盘). Its underlying network engine implements multi-threaded chunked parallel downloading, rapid content-hash upload matching, automated token management, and cloud offline download dispatching.

However, the legacy user presentation layer (`v4.0.2-dev`) had remained largely unchanged since its inception in 2016–2018. Over the last decade, the command-line interface ecosystem has undergone a dramatic aesthetic and architectural renaissance. Modern developer tools—such as GitHub CLI (`gh`), Docker, Kubernetes (`kubectl`), Rclone, and the Charmbracelet suite (`gum`, `glow`, `vhs`, `charm`)—have set a completely new standard for CLI applications:
- Responsive terminal width adaptation.
- Beautiful, declarative Unicode styling with 24-bit TrueColor.
- Dual-mode interfaces providing human-readable interactive feedback alongside first-class machine-readable flags (`--json`, `--plain`).
- Structured error diagnostics that guide users directly toward resolutions.

This document delivers an exhaustive, production-grade redesign proposal for PanGo. It establishes a complete visual, architectural, and operational blueprint to transform the utility into a state-of-the-art terminal application suitable for an Apache-2.0 open-source rebranding.

### 1.2 Comprehensive Defect Inventory of Existing CLI & REPL

A deep-dive forensic audit of the existing codebase (`main.go`, `internal/cliapp/`, `pcstable/`, `pcsliner/`, and `internal/pcsfunctions/`) reveals seven critical deficiencies in the current user experience:

```
┌───────────────────────────────────────────────────────────────────────────────────────┐
│                          LEGACY CLI & REPL UX DEFECT TAXONOMY                         │
├────┬────────────────────────────┬─────────────────────────────────────────────────────┤
│ #  │ Defect Category            │ Concrete Architectural & UX Manifestation           │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D1 │ Visual Identity Void       │ Zero ASCII art logo, branding glyphs, or monogram.  │
│    │                            │ Minimalist 4-line welcome banner with hardcoded 58c │
│    │                            │ divider that does not adapt to screen width.        │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D2 │ Grammar Inconsistencies    │ Arbitrary mix of Unix commands (`ls`, `cd`, `rm`),  │
│    │                            │ flat verbs (`login`, `who`, `quota`, `download`),   │
│    │                            │ and nested commands (`share set`, `offlinedl add`). │
│    │                            │ Single-dash flags (`-bduss=...`) break POSIX rules. │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D3 │ Spanning Table Overflow    │ `pcstable` wraps `tablewriter` with disabled borders│
│    │                            │ and no width clamping. Output spans > 180 cols,     │
│    │                            │ wrapping into unreadable blocks on 80-col screens.  │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D4 │ Readline ANSI Trap         │ `peterh/liner` verifies `unicode.Is(unicode.C, r)`  │
│    │                            │ and crashes with `ErrInvalidPrompt` if ANSI colors  │
│    │                            │ are passed directly into the prompt string.         │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D5 │ Machine-Readable Void      │ Zero `--json` or `--plain` flags across any command.│
│    │                            │ Shell scripts must parse fragile, unstructured text │
│    │                            │ using regex or brittle `awk` scripts.               │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D6 │ Primitive Progress Loops   │ Upload/download progress uses `\r[...] ..........`  │
│    │                            │ with raw dot strings. No Unicode block bars, and    │
│    │                            │ multi-file transfers spam terminal scrollback.      │
├────┼────────────────────────────┼─────────────────────────────────────────────────────┤
│ D7 │ Inconsistent Diagnostics   │ Errors emitted as ad-hoc strings (`fmt.Println(err)`│
│    │                            │ or `遇到错误, 代码: %d`). No structured separation  │
│    │                            │ between Failure, Root Cause, and Actionable Hint.   │
└────┴────────────────────────────┴─────────────────────────────────────────────────────┘
```

#### Detailed Examination of Deficiencies:

1. **Defect D1 — Visual Identity & Brand Void**:
   In `internal/cliapp/repl.go` (lines 102–128), `printWelcomeBanner` simply prints 4 lines of plain text using basic `fatih/color` cyan and green. The prompt provides no brand recognition, omits storage quota metrics (even though the engine has a ready `QuotaInfo()` API), and relies on a rigid 58-character line `──────────────────────────────────────────────────────────` that looks broken on both narrow and wide screens.

2. **Defect D2 — Grammar Inconsistencies & Flag Ergonomics**:
   The current application mixes command styles without clear taxonomy. Shell builtins like `cd` and `ls` operate alongside domain actions (`upload`, `download`) and multi-level subcommands (`share set`, `share list`, `offlinedl query`). Furthermore, flags frequently rely on non-standard single hyphens for long names (`-username=...`), directly violating GNU/POSIX guidelines where single hyphens denote single-character switches (`-u`) and double hyphens denote words (`--username`).

3. **Defect D3 — Spanning Table Overflow**:
   In `pcstable/pcstable.go`, the table constructor sets `tb.SetBorder(false)` and `tb.SetColumnSeparator("")`. When displaying account lists (`loglist`), configuration items (`config`), or deep directory structures (`ls -l`), column output regularly exceeds 180 printable characters. On a standard 80-column terminal, lines wrap arbitrarily across rows, destroying tabular readability.

4. **Defect D4 — Readline ANSI Constraint (`peterh/liner`)**:
   As uncovered in `github.com/peterh/liner@v1.2.2/line.go:605`, `liner` iterates over each rune in the prompt and checks `unicode.Is(unicode.C, r)`. Because ANSI escape sequences begin with ESC (`\x1b`), which is categorized as `unicode.Cc` (control character), passing any colored string into `liner.Prompt()` immediately triggers `ErrInvalidPrompt` ("invalid prompt"). Consequently, developers have been prevented from implementing colored shell prompts without causing runtime crashes.

5. **Defect D5 — Absence of Machine-Readable Output Modes**:
   Commands like `ls`, `search`, `loglist`, and `share list` emit only formatted text intended for human eyes. There is no `--json`, `--tsv`, or `--plain` output option. Automating backups, archiving pipelines, or monitoring scripts via cron requires fragile regex text parsing that breaks whenever output formatting changes.

6. **Defect D6 — Outdated Dot-Based Progress Displays**:
   In `internal/pcsfunctions/pcsdownload/download_task_unit.go:61` and `pcsupload/upload_task_unit.go:65`, progress is rendered as:
   `\r[%s] ↓ %s/%s %s/s in %s, left %s ............`
   This dot string (`............`) offers zero proportional visual feedback compared to modern Unicode block progress bars (`[████████░░░░]`). Furthermore, when concurrent multi-task transfers occur (`load > 1`), each chunk worker prints a new line containing `...\n`, flooding the terminal buffer with hundreds of scrolling lines.

7. **Defect D7 — Inconsistent Diagnostic Hierarchy**:
   When network operations fail (e.g., token expiration, captcha trigger, or rate-limiting), the application outputs raw error codes like `遇到错误, 远程服务器返回错误, 代码: 31045`. Users are left with no explanation of what code `31045` means or how to resolve it (e.g., "BDUSS expired: please run 'login' to re-authenticate").

### 1.3 Strategic Objectives of the Redesign

To address these defects comprehensively, this proposal establishes four core pillars:
1. **Brand Identity & Aesthetic Polish (R1 & R2)**: Introduce a curated set of evocative open-source candidate names, accompanied by meticulously engineered terminal ASCII art logos and responsive banners tested for standard 80-column terminals.
2. **Modern Interactive Shell (R3)**: Architect a Starship-style right-aligned two-line REPL prompt that completely bypasses `liner`'s ANSI restriction while delivering rich context (account name, VIP tier, working directory, and live storage quota).
3. **Responsive Presentation & Output Engine**: Modernize tables with rounded Unicode borders, auto-clamping terminal width detection, block-character progress bars, and dual human/machine output modes (`--json`, `--plain`).
4. **Clean Decoupled Architecture**: Introduce an independent presentation layer (`internal/ui`) powered by `charmbracelet/lipgloss` that preserves 100% backward compatibility with existing command logic, requiring zero breaking changes to core transfer routines.

---

## 2. Comparative Analysis of Popular Go CLI Design Patterns

### 2.1 Benchmark Evaluation Matrix

To establish industry best practices, five prominent Go CLI projects were analyzed across architecture, grammar, visual design tokens, and output formatting:

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                             GO CLI BENCHMARK COMPARATIVE MATRIX                                                  │
├──────────────────┬─────────────────┬────────────────────────────┬─────────────────────────────┬──────────────────┬───────────────┤
│ Tool & Version   │ CLI Framework   │ Command Grammar Pattern    │ Visual Design Hierarchy     │ Output Modes     │ Key Strengths │
├──────────────────┼─────────────────┼────────────────────────────┼─────────────────────────────┼──────────────────┼───────────────┤
│ GitHub CLI       │ `spf13/cobra`   │ Strict Noun-Verb           │ Subtle ANSI (16-color base),│ `--json <fields>`│ Fast, clean,  │
│ (`gh` v2.50+)    │ + `pflag`       │ (`gh repo clone`,          │ status glyphs (`✓`,`✖`,`•`),│ `--jq <expr>`    │ interactive   │
│                  │                 │  `gh pr create`)           │ contextual help hints       │ `--template`     │ fallback, jq  │
├──────────────────┼─────────────────┼────────────────────────────┼─────────────────────────────┼──────────────────┼───────────────┤
│ Rclone           │ `spf13/cobra`   │ Verb-Noun / Unix FS        │ Real-time multi-transfer    │ `lsjson`,        │ Massive multi-│
│ (`rclone` v1.66+)│                 │ (`rclone copy`,            │ status block, live speed,   │ `lsf --format`,  │ remote cloud  │
│                  │                 │  `rclone sync`)            │ ETA, thread statistics      │ `--log-format`   │ engine, stats │
├──────────────────┼─────────────────┼────────────────────────────┼─────────────────────────────┼──────────────────┼───────────────┤
│ Charm Suite      │ `bubbletea`     │ Single-purpose utilities   │ Rounded borders (`╭───╮`),  │ Formatted TTY,   │ World-class   │
│ (`gum`, `glow`)  │ + `lipgloss`    │ (`gum choose`,             │ TrueColor pastel gradients, │ raw markdown,    │ aesthetics,   │
│                  │                 │  `glow document.md`)       │ declarative flexbox layouts │ SVG/GIF output   │ modularity    │
├──────────────────┼─────────────────┼────────────────────────────┼─────────────────────────────┼──────────────────┼───────────────┤
│ Lazygit          │ Custom gocui    │ Modal interactive          │ High-density split panes,   │ Full-screen TUI  │ Keyboard-first│
│ (`lazygit` v0.43)│ fork            │ TUI dashboard              │ active focus glow borders,  │ only (alt screen │ git workflow, │
│                  │                 │                            │ dynamic keybinding footer   │ buffer)          │ dense context │
├──────────────────┼─────────────────┼────────────────────────────┼─────────────────────────────┼──────────────────┼───────────────┤
│ Kubectl          │ `spf13/cobra`   │ Verb-Noun                  │ Bold uppercase headers,     │ `-o json`,       │ Industry      │
│ (`kubectl` v1.30)│                 │ (`kubectl get pods`,       │ left-aligned clean columns, │ `-o yaml`,       │ standard for  │
│                  │                 │  `kubectl describe svc`)   │ standardized status badges  │ `-o wide`        │ automation    │
└──────────────────┴─────────────────┴────────────────────────────┴─────────────────────────────┴──────────────────┴───────────────┘
```

### 2.2 Command Grammar & Hierarchy Patterns

#### Lessons from `gh` and `rclone`:
1. **Noun-Verb vs. Unix Preservation**:
   - `gh` adheres strictly to Noun-Verb (`gh issue list`, `gh pr merge`), making hundreds of operations intuitive to discover.
   - `rclone` retains core Unix verbs (`copy`, `move`, `cat`, `ls`, `mkdir`, `rmdir`) because users think of cloud buckets as filesystems.
   - **Synthesis for PanGo**: Cloud storage users have deep muscle memory for Unix filesystem operations (`ls`, `cd`, `pwd`, `mkdir`, `rm`, `cp`, `mv`). These must remain top-level first-class commands. Conversely, domain operations must be organized into clear hierarchical groups with backward-compatible aliases:
     - `auth`: `auth login`, `auth logout`, `auth whoami`, `auth switch` (aliases: `login`, `logout`, `who`, `su`).
     - `transfer`: `transfer download`, `transfer upload`, `transfer locate` (aliases: `download`, `upload`, `locate`).
     - `share`: `share create`, `share list`, `share cancel` (aliases: `share set`, `share list`).
     - `task`: `task add`, `task list`, `task cancel`, `task clear` (aliases: `offlinedl ...`).
     - `recycle`: `recycle list`, `recycle restore`, `recycle clean`.

2. **POSIX Flag Normalization**:
   - Standardize all flags to support POSIX short flags (`-u`, `-v`, `-h`, `-o`) and long double-dash flags (`--username`, `--verbose`, `--help`, `--output`).
   - Deprecate single-dash long flags (`-username=foo`) with automatic fallback handling.

### 2.3 Visual Tokens & Design Hierarchy

A modern terminal UI relies on semantic design tokens rather than random ANSI color codes. The proposed palette establishes five primary semantic tiers:

```
┌────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                 SEMANTIC DESIGN TOKEN PALETTE                                  │
├────────────────┬──────────────┬───────────────┬──────────────┬───────────────┬─────────────────┤
│ Semantic Role  │ Token Name   │ TrueColor Hex │ ANSI 256 Code│ ANSI 16 Code  │ Primary Use     │
├────────────────┼──────────────┼───────────────┼──────────────┼───────────────┼─────────────────┤
│ Brand Accent   │ `ColorAccent`│ `#00ADD8`     │ `38`         │ `\033[36;1m`  │ Logo, Prompts,  │
│ (Go Cyan)      │              │ (0, 173, 216) │              │ (Bold Cyan)   │ Active Headers  │
├────────────────┼──────────────┼───────────────┼──────────────┼───────────────┼─────────────────┤
│ Success        │ `ColorPass`  │ `#10B981`     │ `48`         │ `\033[32;1m`  │ Downloads OK,   │
│ (Emerald)      │              │ (16, 185, 129)│              │ (Bold Green)  │ Quota OK (<75%) │
├────────────────┼──────────────┼───────────────┼──────────────┼───────────────┼─────────────────┤
│ Warning        │ `ColorWarn`  │ `#F59E0B`     │ `214`        │ `\033[33;1m`  │ Quota > 75%,    │
│ (Amber)        │              │ (245, 158, 11)│              │ (Bold Yellow) │ Retries, Notice │
├────────────────┼──────────────┼───────────────┼──────────────┼───────────────┼─────────────────┤
│ Danger / Error │ `ColorFail`  │ `#EF4444`     │ `196`        │ `\033[31;1m`  │ Network Errors, │
│ (Crimson)      │              │ (239, 68, 68) │              │ (Bold Red)    │ Auth Failures   │
├────────────────┼──────────────┼───────────────┼──────────────┼───────────────┼─────────────────┤
│ Muted / Ghost  │ `ColorMuted` │ `#64748B`     │ `243`        │ `\033[90m`    │ Borders, Footers│
│ (Slate Gray)   │              │ (100, 116, 139│              │ (Hi-Black)    │ Timestamps, Size│
└────────────────┴──────────────┴───────────────┴──────────────┴───────────────┴─────────────────┘
```

#### Typographic Weight Discipline:
- **Bold (`\033[1m`)**: Applied strictly to key entity names (file names, user IDs, table headers, command keywords).
- **Normal / Regular**: Used for informational values, table cell contents, and metrics.
- **Dim / Faint (`\033[2m`)**: Used for secondary metadata (file permissions, timestamps, divider lines, and navigation hints).

### 2.4 Interactive Shell & REPL Aesthetics

Modern CLI shells (such as `gh`, Starship, Fish, and Charm tools) provide instant situational awareness. When entering an interactive session, the user should immediately perceive:
1. **Who am I?** (Active user account, VIP tier).
2. **Where am I?** (Current cloud working directory).
3. **What is my capacity?** (Live graphical quota bar: used vs. total storage).
4. **Is the session ready?** (Distinct prompt indicator ready for input).

### 2.5 Dual Output Modes (Human vs Machine-Readable Automation)

The design must enforce strict separation between interactive human sessions and non-interactive script pipelines:
- **Interactive TTY Mode** (`isatty.IsTerminal(os.Stdout.Fd()) == true`):
  Emits formatted tables with rounded Unicode borders, live progress bars, colored status badges, and spinners.
- **Pipeline / Machine Mode** (`!isatty.IsTerminal` or `--plain` / `--json`):
  - `--json`: Outputs clean, valid JSON structures directly to stdout. Informational messages and banners are routed to stderr or suppressed.
  - `--plain`: Outputs tab-separated values (TSV) without ANSI escape codes, box-drawing characters, or header decorations, optimized for consumption by `awk`, `cut`, `xargs`, or `grep`.
  - `--quiet, -q`: Completely silences progress bars and banner text, emitting only the explicit result (e.g., download file path or share link URL).

---

## 3. Project Naming Recommendations (Requirement R1)

### 3.1 Strategic Rebranding Rationale

The legacy project name has served the community since 2016. However, as an open-source tool evolving under the Apache-2.0 license, a modern rebranding to `PanGo` (`pango`) is vital for three strategic reasons:
1. **Trademark Decoupling & Risk Mitigation**: The name "Baidu" is a registered commercial trademark. Adopting an independent open-source moniker protects maintainers, packagers (Homebrew, Arch AUR, Debian), and downstream forks from intellectual property friction.
2. **CLI Ergonomics & Brevity**: Typing a hyphenated mixed-case binary name or creating custom aliases (`alias pcs=...`) is cumbersome. Modern developer tools feature short, punchy binary names of 4 to 6 characters (e.g., `gh`, `kumo`, `pacer`, `pango`).
3. **Identity Evolution**: The project is no longer just a raw PCS protocol scraper. It has evolved into a sophisticated, multi-threaded personal cloud storage orchestration engine.

### 3.2 Project Naming Evaluation Matrix

Six distinct naming candidates were curated across different brand archetypes:

```
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                           PROJECT NAMING MATRIX                                                  │
├───────────────┬────────────────────────────┬─────────────┬──────────────┬────────────────────────────────────────┤
│ Candidate     │ Archetype / Style          │ CLI Binary  │ Repo Slug    │ Brand Identity Essence                 │
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 1. CloudPacer │ Fast / Turbo Sync Engine   │ `pacer`     │ `cloudpacer` │ High-speed throughput, steady tempo    │
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 2. PanFlux    │ Fluid Pipeline & Stream    │ `pflux`     │ `panflux`    │ Frictionless data stream & sync        │
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 3. KumoCLI    │ Minimalist Developer Tool  │ `kumo`      │ `kumo`       │ Zen elegance, lightness, dotfile-ready │
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 4. DrivePilot │ Autonomous Cockpit         │ `pilot`     │ `drivepilot` │ Telemetry, navigation, automated flight│
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 5. AetherPan  │ Universal Cloud Fabric     │ `aether`    │ `aetherpan`  │ Resilient, celestial, archival gateway │
├───────────────┼────────────────────────────┼─────────────┼──────────────┼────────────────────────────────────────┤
│ 6. PanGo      │ Community Continuation     │ `pango`     │ `pango`      │ Warm, enthusiastic, instant recognition│
└───────────────┴────────────────────────────┴─────────────┴──────────────┴────────────────────────────────────────┘
```

### 3.3 Detailed Candidate Profiles

#### Candidate 1: CloudPacer (`pacer`)
- **Naming Archetype**: High-Performance Turbo Transfer / Cadence Leader.
- **CLI Binary Name**: `pacer` (5 keystrokes).
- **Repository Slug**: `github.com/cloudpacer/cloudpacer` (or `pacer-cli`).
- **Rationale**: In marathon racing and cycling, a "pacer" is the elite runner who sets the blistering, consistent tempo for the pack. The defining core capability of PanGo has always been its extraordinary multi-threaded chunk download engine and instantaneous hash-matching rapid upload, outperforming browsers and official desktop clients by orders of magnitude. "CloudPacer" communicates relentless speed, high-cadence data movement, and rock-solid transfer stability.
- **Brand Identity**:
  - *Tagline*: *"High-performance cloud storage orchestration at your pace."*
  - *Target Persona*: NAS power users, homelab curators, backup automation engineers, and users operating high-bandwidth gigabit connections who demand full network saturation.
  - *Visual Aesthetic*: Racing cyan (`#00E5FF`) and high-voltage amber (`#FFB300`), aerodynamic slanted ASCII logos, telemetry speed gauges.

#### Candidate 2: PanFlux (`pflux`)
- **Naming Archetype**: Fluid Cloud Stream & Dynamic Pipeline.
- **CLI Binary Name**: `pflux` (or `flux`).
- **Repository Slug**: `github.com/panflux/panflux`.
- **Rationale**: In physics and vector calculus, "flux" defines the rate of flow of a physical property or field through a surface. "Pan" grounds the utility in the Netdisk (网盘) ecosystem, while "Flux" signifies continuous, frictionless streaming of multi-gigabyte files between local NVMe storage and remote cloud object stores.
- **Brand Identity**:
  - *Tagline*: *"Seamless, high-throughput data flux for personal cloud storage."*
  - *Target Persona*: Sysadmins, data backup operators, and DevOps engineers piping data directly from Unix standard input/output into cloud storage.
  - *Visual Aesthetic*: Gradient teal to deep neon blue (`#06B6D4` → `#3B82F6`), wave/ripple ASCII glyphs, fluid dynamic progress bars.

#### Candidate 3: KumoCLI (`kumo`)
- **Naming Archetype**: Minimalist & Elegant Developer Utility.
- **CLI Binary Name**: `kumo` (4 keystrokes, effortless terminal ergonomics).
- **Repository Slug**: `github.com/kumo-cli/kumo` (or `kumopcs`).
- **Rationale**: "Kumo" (雲) is the Japanese word for "cloud". It evokes lightness, zen-like minimalism, and whisper-quiet efficiency, following the design ethos of revered modern CLI utilities (like `bat`, `eza`, `fzf`, `zoxide`). Furthermore, it serves as an appreciative homage to the original author's handle ("iikira").
- **Brand Identity**:
  - *Tagline*: *"Aesthetic, lightweight, and blazing-fast cloud drive terminal."*
  - *Target Persona*: Modern Unix terminal artisans, dotfile minimalists, macOS and Linux developers who value typography, instant startup time, and clean visual design.
  - *Visual Aesthetic*: Soft indigo (`#6366F1`) and mist white (`#F8FAFC`), rounded Unicode box borders (`╭─╮`), refined Asian cloud motifs.

#### Candidate 4: DrivePilot (`pilot`)
- **Naming Archetype**: Autonomous Cockpit & Precision Navigator.
- **CLI Binary Name**: `pilot` (or `dpilot`).
- **Repository Slug**: `github.com/drivepilot/pilot`.
- **Rationale**: With recursive directory traversal, tab completion, offline download queues, and remote share link resolution, PanGo acts as a sophisticated flight deck for cloud assets. "DrivePilot" conveys cockpit telemetry, intelligent routing, and automated autopilot execution of complex cloud operations.
- **Brand Identity**:
  - *Tagline*: *"Your command-line cockpit for cloud drive navigation."*
  - *Target Persona*: Engineers managing millions of files across distributed directories who prefer an interactive cockpit over memorizing complex flag combinations.
  - *Visual Aesthetic*: Aviation emerald (`#10B981`) and deep radar slate (`#0F172A`), bracketed HUD telemetry badges (`[ALT: ONLINE]`, `[SPEED: 48MB/s]`).

#### Candidate 5: AetherPan (`aether`)
- **Naming Archetype**: Universal Cloud Fabric & Celestial Archival Gateway.
- **CLI Binary Name**: `aether` (or `apan`).
- **Repository Slug**: `github.com/aetherpan/aetherpan`.
- **Rationale**: In classical antiquity, "Aether" is the pure, luminous fifth element that fills the celestial universe above the terrestrial sphere. In computing, it symbolizes an omnipresent, resilient transmission medium. "AetherPan" elevates the software from a script into an enterprise-grade, resilient cloud asset manager built for permanent archiving.
- **Brand Identity**:
  - *Tagline*: *"The boundless terminal gateway to your cloud assets."*
  - *Target Persona*: Digital preservationists, research archivists, and enterprise homelab custodians seeking maximum durability and clean open-source lineage.
  - *Visual Aesthetic*: Celestial purple (`#A855F7`) and starlight cyan (`#38BDF8`), cosmic ASCII symbols (`✦`, `◈`, `▲`), high-contrast framed panels.

#### Candidate 6: PanGo (`pango`)
- **Naming Archetype**: Friendly Community Evolution & Direct Continuation.
- **CLI Binary Name**: `pango`.
- **Repository Slug**: `github.com/pango-cli/pango`.
- **Rationale**: A natural triple pun combining "Pan" (网盘), "Go" (the implementation language), and the enthusiastic action call "Let's Go!". It provides immediate name recognition to community users while shedding trademark encumbrances.
- **Brand Identity**:
  - *Tagline*: *"Go faster, go simpler: the modern Baidu Pan terminal client."*
  - *Target Persona*: Community users adopting the modernized PanGo client.
  - *Visual Aesthetic*: Go Gopher Cyan (`#00ADD8`) and vibrant green (`#22C55E`), clean, friendly, and approachable.

### 3.4 Final Maintainer Recommendation

- **Primary Recommendation**: **CloudPacer** (CLI: `pacer`). It best represents the software's core competitive advantage—blistering, multi-threaded transfer speeds.
- **Secondary Recommendation (Minimalist Track)**: **KumoCLI** (CLI: `kumo`). The ideal selection if the maintainers wish to align with the modern Charmbracelet/Rust CLI aesthetic.
- **Secondary Recommendation (Heritage Track)**: **PanGo** (CLI: `pango`). The natural bridge for preserving maximum community continuity.

---

## 4. Terminal ASCII Art Logo Designs (Requirement R2)

### 4.1 Terminal Width Constraint & Mathematics

A critical design requirement for terminal applications is strict adherence to the **80-column standard width limit** (POSIX `80x24` VT100 standard). If any logo or border exceeds 80 character cells, terminal emulators will wrap the trailing characters to the next row, producing jagged, unreadable visual corruption.

```
0                   10                  20                  30                  40                  50                  60                  70        76  80
|-------------------|-------------------|-------------------|-------------------|-------------------|-------------------|-------------------|---------|---|
[ Left Margin (2c) ] [                  Usable Card Width (72c)                                                ] [ Right Margin (4c) ]
  ╭────────────────────────────────────────────────────────────────────────╮  <-- Exact width: 76 cols
  │  Logo / Content Area: Maximum width <= 64 cols                         │
  ╰────────────────────────────────────────────────────────────────────────╯
```

#### The Layout Formula:
$$\text{Total Outer Width} = \text{Left Margin (2)} + \text{Left Border (2)} + \text{Inner Width (70)} + \text{Right Border (2)} = 76 \text{ columns}$$
All four candidate logos have been mathematically designed and verified to remain **$\le 63$ columns**, leaving at least 17 columns of safety margin within an 80-column terminal.

---

### 4.2 Candidate 1: "CloudVault / CloudPCS" (Modern Cloud & Storage Vault)

- **Style**: Modern Cloud Silhouette with Slant Block Wordmark.
- **Dimensions**: Width = 61 columns, Height = 7 lines.
- **Safety Margin**: 19 columns under the 80-column limit.

#### A. Monochrome Raw ASCII Art Preview
```text
      .---.       ____ _                 _ ____   ____ ____  
     (     )     / ___| | ___  _   _  __| |  _ \ / ___/ ___| 
    (  . .  )   | |   | |/ _ \| | | |/ _` | |_) | |   \___ \ 
   (__( _ )__)  | |___| | (_) | |_| | (_| |  __/| |___ ___) |
    /  /  /  /   \____|_|\___/ \__,_|\__,_|_|    \____|____/ 
                ============================================
                »» Personal Cloud Storage & Sync Engine ««  
```

#### B. Pure ASCII Fallback Preview (Zero Unicode)
```text
      .---.       ____ _                 _ ____   ____ ____  
     (     )     / ___| | ___  _   _  __| |  _ \ / ___/ ___| 
    (  . .  )   | |   | |/ _ \| | | |/ _` | |_) | |   \___ \ 
   (__( _ )__)  | |___| | (_) | |_| | (_| |  __/| |___ ___) |
    /  /  /  /   \____|_|\___/ \__,_|\__,_|_|    \____|____/ 
                ============================================
                >> Personal Cloud Storage & Sync Engine <<  
```

#### C. Color Palette Specification: "Azure Sky & Ice Silver"
```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                   CANDIDATE 1 COLOR PALETTE MATRIX                                      │
├────────────────────┬──────────────────────┬───────────────┬────────────────┬──────────────┬─────────────┤
│ Visual Element     │ Semantic Role        │ TrueColor Hex │ RGB Integers   │ ANSI 256 Code│ ANSI 16 Code│
├────────────────────┼──────────────────────┼───────────────┼────────────────┼──────────────┼─────────────┤
│ Cloud Silhouette   │ Contour Highlighting │ `#F0F9FF`     │ (240, 249, 255)│ `255`        │ `\033[97m`  │
│ Rain / Data Drops  │ High-Speed Data Flow │ `#38BDF8`     │ (56, 189, 248) │ `117`        │ `\033[96m`  │
│ Brand Wordmark     │ Primary Logo Name    │ `#0284C7`     │ (2, 132, 199)  │ `32`         │ `\033[34;1m`│
│ Accent Divider     │ Structural Rule      │ `#64748B`     │ (100, 116, 139)│ `243`        │ `\033[90m`  │
│ Subtitle Tagline   │ Secondary Text       │ `#94A3B8`     │ (148, 163, 184)│ `248`        │ `\033[37m`  │
└────────────────────┴──────────────────────┴───────────────┴────────────────┴──────────────┴─────────────┘
```

#### D. Exact ANSI Colored Escape Sequence Stream
```bash
\033[38;2;240;249;255m      .---.       \033[38;2;2;132;199m____ _                 _ ____   ____ ____  \033[0m
\033[38;2;240;249;255m     (     )     \033[38;2;2;132;199m/ ___| | ___  _   _  __| |  _ \ / ___/ ___| \033[0m
\033[38;2;240;249;255m    (  . .  )   \033[38;2;2;132;199m| |   | |/ _ \| | | |/ _` | |_) | |   \___ \ \033[0m
\033[38;2;240;249;255m   (__( _ )__)  \033[38;2;2;132;199m| |___| | (_) | |_| | (_| |  __/| |___ ___) |\033[0m
\033[38;2;56;189;248m    /  /  /  /   \033[38;2;2;132;199m\____|_|\___/ \__,_|\__,_|_|    \____|____/ \033[0m
\033[38;2;100;116;139m                ============================================\033[0m
\033[38;2;148;163;184m                »» Personal Cloud Storage & Sync Engine ««  \033[0m
```

---

### 4.3 Candidate 2: "VortexTransfer / FastPCS" (High-Speed Turbo Transfer)

- **Style**: Forward-Leaning Italicized Slant with Dynamic Speed Streaks and High-Voltage Bolts.
- **Dimensions**: Width = 63 columns, Height = 6 lines.
- **Safety Margin**: 17 columns under the 80-column limit.

#### A. Monochrome Raw ASCII Art Preview
```text
    ⚡   ______ ___    _____ ______ ____  ______ _____         
   //  / ____//   |  / ___//_  __// __ \/ ____// ___/   //    
  //  / /_   / /| |  \__ \  / /  / /_/ / /     \__ \   //     
 //  / __/  / ___ | ___/ / / /  / ____/ /___  ___/ /  //      
 /  /_/    /_/  |_|/____/ /_/  /_/    \____/ /____/  //   ⚡  
   »»» HIGH-SPEED TURBO MULTI-THREAD TRANSFER ENGINE «««      
```

#### B. Pure ASCII Fallback Preview (Zero Unicode)
```text
   [>]  ______ ___    _____ ______ ____  ______ _____         
  //   / ____//   |  / ___//_  __// __ \/ ____// ___/   //    
 //   / /_   / /| |  \__ \  / /  / /_/ / /     \__ \   //     
//   / __/  / ___ | ___/ / / /  / ____/ /___  ___/ /  //      
/   /_/    /_/  |_|/____/ /_/  /_/    \____/ /____/  //   [<] 
   >>> HIGH-SPEED TURBO MULTI-THREAD TRANSFER ENGINE <<<      
```

#### C. Color Palette Specification: "Cyber Neon & High-Voltage Amber"
```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                   CANDIDATE 2 COLOR PALETTE MATRIX                                      │
├────────────────────┬──────────────────────┬───────────────┬────────────────┬──────────────┬─────────────┤
│ Visual Element     │ Semantic Role        │ TrueColor Hex │ RGB Integers   │ ANSI 256 Code│ ANSI 16 Code│
├────────────────────┼──────────────────────┼───────────────┼────────────────┼──────────────┼─────────────┤
│ Turbo Bolts (`⚡`) │ Speed Energy Glyphs  │ `#F59E0B`     │ (245, 158, 11) │ `214`        │ `\033[93m`  │
│ Speed Streaks (`//`) Dynamic Acceleration │ `#06B6D4`     │ (6, 182, 212)  │ `45`         │ `\033[96m`  │
│ Main Typography    │ Slanted Wordmark     │ `#D946EF`     │ (217, 70, 239) │ `169`        │ `\033[95;1m`│
│ Tagline Accent     │ Performance Banner   │ `#FBBF24`     │ (251, 191, 36) │ `215`        │ `\033[33m`  │
│ Outer Frame Tone   │ Structural Contrast  │ `#334155`     │ (51, 65, 85)   │ `238`        │ `\033[90m`  │
└────────────────────┴──────────────────────┴───────────────┴────────────────┴──────────────┴─────────────┘
```

#### D. Exact ANSI Colored Escape Sequence Stream
```bash
\033[38;2;245;158;11m    ⚡   \033[38;2;217;70;239m______ ___    _____ ______ ____  ______ _____         \033[0m
\033[38;2;6;182;212m   //  \033[38;2;217;70;239m/ ____//   |  / ___//_  __// __ \/ ____// ___/   \033[38;2;6;182;212m//    \033[0m
\033[38;2;6;182;212m  //  \033[38;2;217;70;239m/ /_   / /| |  \__ \  / /  / /_/ / /     \__ \   \033[38;2;6;182;212m//     \033[0m
\033[38;2;6;182;212m //  \033[38;2;217;70;239m/ __/  / ___ | ___/ / / /  / ____/ /___  ___/ /  \033[38;2;6;182;212m//      \033[0m
\033[38;2;6;182;212m /  \033[38;2;217;70;239m/_/    /_/  |_|/____/ /_/  /_/    \____/ /____/  \033[38;2;6;182;212m//   \033[38;2;245;158;11m⚡  \033[0m
\033[38;2;251;191;36m   »»» HIGH-SPEED TURBO MULTI-THREAD TRANSFER ENGINE «««      \033[0m
```

---

### 4.4 Candidate 3: "PanGo / Modern Geometric" (Charm TUI Minimalist Monogram)

- **Style**: Ultra-Clean Geometric Block Monogram with Nordic Minimalist Typography.
- **Dimensions**: Width = 50 columns, Height = 4 lines.
- **Safety Margin**: 30 columns under the 80-column limit (ideal for side-by-side split banners).

#### A. Monochrome Raw ASCII Art Preview
```text
   ▄▄▄▄▄▄▄   █▀█ ▄▀█ █▄ █ █▀▀ █▀█                 
   █ ▄ ▄ █   █▀▀ █▀█ █ ▀█ █▄█ █▄█  ·  P C S       
   █ █ █ █  ───────────────────────────────────── 
   ▀▀▀▀▀▀▀   Modern Terminal Cloud Storage Engine 
```

#### B. Pure ASCII Fallback Preview (Zero Unicode)
```text
  +-------+   ____   _    _   _  ____  ___           
  | [=]   |  |  _ \ / \  | \ | |/ ___|/ _ \  · PCS · 
  |   [=] |  | |_) / _ \ |  \| | |  _| | | |         
  | [=]   |  |  __/ ___ \| |\  | |_| | |_| |         
  +-------+  |_| /_/   \_\_| \_|\____|\___/          
  -------------------------------------------------- 
             Modern Terminal Cloud Storage Engine    
```

#### C. Color Palette Specification: "Nordic Mint & Deep Obsidian"
```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                   CANDIDATE 3 COLOR PALETTE MATRIX                                      │
├────────────────────┬──────────────────────┬───────────────┬────────────────┬──────────────┬─────────────┤
│ Visual Element     │ Semantic Role        │ TrueColor Hex │ RGB Integers   │ ANSI 256 Code│ ANSI 16 Code│
├────────────────────┼──────────────────────┼───────────────┼────────────────┼──────────────┼─────────────┤
│ Vault Monogram     │ Outer Boundary Frame │ `#0F766E`     │ (15, 118, 110) │ `30`         │ `\033[36m`  │
│ Monogram Core      │ Storage Unit Blocks  │ `#F8FAFC`     │ (248, 250, 252)│ `231`        │ `\033[97;1m`│
│ Brand Lettering    │ Emerald Block Type   │ `#10B981`     │ (16, 185, 129) │ `48`         │ `\033[92;1m`│
│ Secondary Tag      │ "PCS" Edition Label  │ `#34D399`     │ (52, 211, 153) │ `78`         │ `\033[92m`  │
│ Divider Line       │ Hairline Rule        │ `#334155`     │ (51, 65, 85)   │ `238`        │ `\033[90m`  │
│ Subtitle Tagline   │ Descriptive Subtitle │ `#94A3B8`     │ (148, 163, 184)│ `248`        │ `\033[37m`  │
└────────────────────┴──────────────────────┴───────────────┴────────────────┴──────────────┴─────────────┘
```

#### D. Exact ANSI Colored Escape Sequence Stream
```bash
\033[38;2;15;118;110m   ▄▄▄▄▄▄▄   \033[38;2;16;185;129m█▀█ ▄▀█ █▄ █ █▀▀ █▀█                 \033[0m
\033[38;2;15;118;110m   █ \033[38;2;248;250;252m▄ ▄\033[38;2;15;118;110m █   \033[38;2;16;185;129m█▀▀ █▀█ █ ▀█ █▄█ █▄█  \033[38;2;52;211;153m·  P C S       \033[0m
\033[38;2;15;118;110m   █ \033[38;2;248;250;252m█ █\033[38;2;15;118;110m █  \033[38;2;51;65;85m───────────────────────────────────── \033[0m
\033[38;2;15;118;110m   ▀▀▀▀▀▀▀   \033[38;2;148;163;184mModern Terminal Cloud Storage Engine \033[0m
```

---

### 4.5 Candidate 4: "PanGo Classic Heritage" (Retro Terminal Block)

- **Style**: Retro Standard ANSI Block Lettering.
- **Dimensions**: Width = 63 columns, Height = 7 lines.
- **Safety Margin**: 17 columns under the 80-column limit.

#### Monochrome Raw ASCII Art Preview
```text
  ____        _     _       ____   ____ ____        ____       
 | __ )  __ _(_) __| |_   _|  _ \ / ___/ ___|      / ___| ___  
 |  _ \ / _` | |/ _` | | | | |_) | |   \___ \ ___ | |  _ / _ \ 
 | |_) | (_| | | (_| | |_| |  __/| |___ ___) |___|| |_| | (_) |
 |____/ \__,_|_|\__,_|\__,_|_|    \____|____/      \____|\___/ 
 ───────────────────────────────────────────────────────────── 
      Open-Source Baidu Netdisk Terminal Client & REPL         
```

---

### 4.6 Verification of Standard Terminal Width (<= 80 Columns)

Each candidate logo was programmatically validated by calculating character cell widths:

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                              ASCII LOGO WIDTH VERIFICATION                             │
├───────────────────────────────────┬──────────────┬──────────────┬──────────────────────┤
│ Candidate Logo                    │ Max Columns  │ Safety Margin│ Status (<= 80 Cols)  │
├───────────────────────────────────┼──────────────┼──────────────┼──────────────────────┤
│ Candidate 1: CloudVault           │ 61 columns   │ +19 cols     │ PASS (No-Wrap)       │
│ Candidate 2: VortexTransfer       │ 63 columns   │ +17 cols     │ PASS (No-Wrap)       │
│ Candidate 3: PanGo Geometric      │ 50 columns   │ +30 cols     │ PASS (No-Wrap)       │
│ Candidate 4: PanGo Classic        │ 63 columns   │ +17 cols     │ PASS (No-Wrap)       │
└───────────────────────────────────┴──────────────┴──────────────┴──────────────────────┘
```

> **CJK Terminal Width Guidance**:
> In Unicode Standard Annex #11 (UAX #11), box-drawing characters (`─`) and block runes (`█`) carry an East Asian Ambiguous (`A`) width property. In CJK terminal environments configured with ambiguous character width equal to 2 (`runewidth.DefaultCondition.EastAsianWidth = true`), layouts containing horizontal box lines or block elements (such as Candidates 3 and 4) can expand in column width. For terminal configurations where ambiguous width equals 2, **Candidate 1 ("CloudVault")**, **Candidate 2 ("VortexTransfer")**, and **Mode E (Pure ASCII Fallback)** are recommended, as their character sets contain zero ambiguous-width runes and guarantee strictly invariant column widths across all terminal locales.

---

## 5. Modern CLI & Interactive Shell UI/UX Redesign Specification (Requirement R3)

### 5.1 Responsive Startup Banner Specification

The startup banner replaces the legacy 4-line greeting with a responsive presentation engine supporting five distinct operational modes:

#### Mode A: Standard 80-Column Stacked Box Layout (76 Cols Exact)
Designed specifically for standard 80x24 terminal emulators. Uses rounded box-drawing characters (`╭─╮`, `│ │`, `╰─╯`) and embeds account metadata alongside a live visual storage bar.

##### Visual Preview — Authenticated Active Session:
```text
  ╭────────────────────────────────────────────────────────────────────────╮
  │      .---.       ____ _                 _ ____   ____ ____             │
  │     (     )     / ___| | ___  _   _  __| |  _ \ / ___/ ___|            │
  │    (  . .  )   | |   | |/ _ \| | | |/ _` | |_) | |   \___ \            │
  │   (__( _ )__)  | |___| | (_) | |_| | (_| |  __/| |___ ___) |           │
  │    /  /  /  /   \____|_|\___/ \__,_|\__,_|_|    \____|____/            │
  │                 »» Personal Cloud Storage & Sync Engine ««             │
  ├────────────────────────────────────────────────────────────────────────┤
  │  Version: v4.1.0 (commit a9b2c3d)         Arch: linux/amd64 (go1.23)   │
  │  Account: ● active (UID: 882910481)       Workdir: /我的资源/Docs      │
  │  Storage: [██████████████░░░░░░░░░░░░] 54.2% (1,084.5 GB / 2.0 TB)     │
  ├────────────────────────────────────────────────────────────────────────┤
  │  Commands: ls, cd, download, upload, share, offlinedl, quota, config   │
  │  Help: 'help [cmd]' for manual  |  'exit' to quit  |  Tab to complete  │
  ╰────────────────────────────────────────────────────────────────────────╯
```

##### Visual Preview — Unauthenticated / Logged Out Session:
```text
  ╭────────────────────────────────────────────────────────────────────────╮
  │      .---.       ____ _                 _ ____   ____ ____             │
  │     (     )     / ___| | ___  _   _  __| |  _ \ / ___/ ___|            │
  │    (  . .  )   | |   | |/ _ \| | | |/ _` | |_) | |   \___ \            │
  │   (__( _ )__)  | |___| | (_) | |_| | (_| |  __/| |___ ___) |           │
  │    /  /  /  /   \____|_|\___/ \__,_|\__,_|_|    \____|____/            │
  │                 »» Personal Cloud Storage & Sync Engine ««             │
  ├────────────────────────────────────────────────────────────────────────┤
  │  Version: v4.1.0 (commit a9b2c3d)         Arch: linux/amd64 (go1.23)   │
  │  Account: ○ 未登录 (Not Logged In)        Workdir: (none)              │
  │  Storage: [░░░░░░░░░░░░░░░░░░░░░░░░░░]   0.0% (Sign in to view quota)  │
  ├────────────────────────────────────────────────────────────────────────┤
  │  Commands: login, help, config, exit                                   │
  │  Notice: Run 'login' to sign in with BDUSS or cookie credentials       │
  ╰────────────────────────────────────────────────────────────────────────╯
```

---

#### Mode B: Wide Terminal Side-by-Side 2-Column Split Layout (111 Cols Exact)
When terminal width exceeds 100 columns, the layout shifts to a high-density side-by-side format. By placing the brand monogram on the left and telemetry cards on the right, vertical consumption is reduced from 16 rows to just 8 rows.

##### Visual Preview:
```text
  ╭──────────────────────────────────────────────────┬────────────────────────────────────────────────────────╮
  │    ▄▄▄▄▄▄▄   █▀█ ▄▀█ █▄ █ █▀▀ █▀█                │ ● Account: baidu_vip_user (UID: 882910481)             │
  │    █ ▄ ▄ █   █▀▀ █▀█ █ ▀█ █▄█ █▄█  · PCS         │ 📁 Workdir: /我的资源/Work/                            │
  │    █ █ █ █  ─────────────────────────────        │ 📊 Storage: [██████████████░░░░░░░░░░░░] 54.2%         │
  │    ▀▀▀▀▀▀▀   Next-Gen Personal Cloud Sync        │    Usage:   1,084.5 GB / 2,000.0 GB (915.5 GB free)    │
  │    Engine:   v4.1.0-rev.a9b2c (linux/amd64)      │ ⚡ Speed:   Turbo Multi-Thread Accelerated             │
  │    ProTip:   'd -p 10 <path>' for turbo dl       │ ⌨  Help:    'help [cmd]' for manual  |  'exit' quit    │
  ╰──────────────────────────────────────────────────┴────────────────────────────────────────────────────────╯
```

---

#### Mode C: Narrow / Compact View (< 70 Cols)
When invoked in narrow windows, tmux vertical splits, or mobile SSH clients (Termius, JuiceSSH), box borders are omitted to prevent line-wrapping:
```text
  [PanGo PCS] v4.1.0 (linux/amd64)
  Account: baidu_vip_user | Quota: 54.2% [1.08 TB / 2.00 TB]
  Workdir: /我的资源/Work/
  Type 'help' for commands, 'exit' to quit.
```

---

#### Mode D: Headless / Script / Pipe Mode
When standard output is redirected (`!isatty.IsTerminal(os.Stdout.Fd())`) or `--quiet` is passed, the banner is **completely suppressed (0 lines rendered)**.

---

#### Mode E: `NO_COLOR` Compliance & Pure ASCII Fallback
In accordance with the open-source [no-color.org](https://no-color.org) standard:
- If `os.Getenv("NO_COLOR") != ""`, all ANSI color codes are stripped.
- In ASCII-only environments (`LANG=C`), Unicode box characters degrade to `+`, `-`, and `|`:
```text
  +------------------------------------------------------------------------+
  |      .---.       ____ _                 _ ____   ____ ____             |
  |     (     )     / ___| | ___  _   _  __| |  _ \ / ___/ ___|            |
  |    (  . .  )   | |   | |/ _ \| | | |/ _` | |_) | |   \___ \            |
  |   (__( _ )__)  | |___| | (_) | |_| | (_| |  __/| |___ ___) |           |
  |    /  /  /  /   \____|_|\___/ \__,_|\__,_|_|    \____|____/            |
  |                 >> Personal Cloud Storage & Sync Engine <<             |
  +------------------------------------------------------------------------+
  |  Version: v4.1.0 (commit a9b2c3d)         Arch: linux/amd64 (go1.23)   |
  |  Account: * active (UID: 882910481)       Workdir: /我的资源/Docs      |
  |  Storage: [==============............] 54.2% (1,084.5 GB / 2.0 TB)     |
  +------------------------------------------------------------------------+
  |  Commands: ls, cd, download, upload, share, offlinedl, quota, config   |
  |  Help: 'help [cmd]' for manual  |  'exit' to quit  |  Tab to complete  |
  +------------------------------------------------------------------------+
```

---

### 5.2 Interactive Shell REPL Prompt Redesign

#### Architectural Discovery: Liner's ANSI Restriction
A critical finding during codebase exploration is that `peterh/liner` strictly prohibits ANSI color escape sequences in prompts. As implemented in `line.go:605`:
```go
func (s *State) PromptWithSuggestion(prompt string, text string, pos int) (string, error) {
    for _, r := range prompt {
        if unicode.Is(unicode.C, r) {
            return "", ErrInvalidPrompt
        }
    }
    ...
}
```
Because the escape character `\x1b` is categorized under `unicode.Cc` (control characters), passing any colored string directly into `line.Prompt()` causes `liner` to return `ErrInvalidPrompt` immediately.

#### Configurable Two-Line REPL Prompt Architecture (`starship` & `boxed`)
To bypass this limitation without introducing unstable readline replacements, `pan-tools` implements the **Two-Line Decoupled Prompt Pattern** (`internal/ui/prompt.go`) with two user-selectable styles (`pant config set -prompt_style <starship|boxed>`, `PANT_PROMPT_STYLE`, or `--prompt-style`):

##### Style 1: `starship` (Default — Right-Aligned Minimalist)
```
[ Line 1: Right-Aligned Context Bar ] --> Printed directly to os.Stdout via Lipgloss
alice  SVIP 10  ~/工作/架构设计                              54.2% · 1.1TB/2.0TB

[ Line 2: Printable Readline Symbol ] --> Passed into liner.State.Prompt(prompt)
❯ 
```

##### Style 2: `boxed` (Rounded Box-Connector Frame)
```
[ Line 1: Framed Brand & Tag Bar    ] --> Printed directly to os.Stdout via Lipgloss
╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]

[ Line 2: Printable Box Connector   ] --> Passed into liner.State.Prompt(prompt)
╰─❯ 
```

##### Pure ASCII Fallback (`--ascii` / `PANT_ASCII=1`):
```text
# starship:
alice  [SVIP 10]  ~/工作/架构设计                            54.2% | 1.1TB/2.0TB
> 

# boxed:
+- pan-tools | * alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]
+-> 
```

##### Why This Architecture Is Optimal:
1. **Zero ANSI Control Characters in `liner`**: Line 2 contains only printable runes (`❯ ` or `╰─❯ `, degrading under `--ascii` to `> ` or `+-> `). `unicode.Is(unicode.C, r)` evaluates to `false` for every rune, so `liner` never triggers `ErrInvalidPrompt`.
2. **Flawless Cursor Positioning**: Because Line 2 has zero invisible escape sequences, `liner` calculates character widths and cursor offsets with 100% precision. Horizontal cursor jumping and line ghosting are completely eliminated.
3. **Nerd-Font-Free High Density**: Line 1 renders clean Atom One Dark pills (username, VIP/SVIP badge, shortened `~/...` cloud path on the left; quota usage ratio & capacity right-aligned to terminal width) without requiring patched Nerd Fonts.

#### Contextual Prompt States & Tab Autocompletion

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                               CONTEXTUAL PROMPT STATES                                 │
├───────────────────────┬────────────────────────────────────────────────────────────────┤
│ State                 │ Prompt Rendering                                               │
├───────────────────────┼────────────────────────────────────────────────────────────────┤
│ 1. Active User        │ alice  FREE  ~/我的资源                     54.2% · 1.1TB/2.0TB│
│    (Standard)         │ ❯                                                              │
├───────────────────────┼────────────────────────────────────────────────────────────────┤
│ 2. Unauthenticated    │ pan-tools  ○ 未登录 (输入 login 登录)                          │
│    (Guest)            │ ❯                                                              │
├───────────────────────┼────────────────────────────────────────────────────────────────┤
│ 3. SVIP High Speed    │ alice  SVIP 10  ~                            88.1% · 8.8TB/10TB│
│    (VIP Mode)         │ ❯                                                              │
├───────────────────────┼────────────────────────────────────────────────────────────────┤
│ 4. Boxed Frame Style  │ ╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [54%]   │
│    (`boxed` Option)   │ ╰─❯                                                            │
└───────────────────────┴────────────────────────────────────────────────────────────────┘
```

##### Autocompletion Enhancements:
- **Directory Visual Indication**: Append trailing slashes (`/`) to remote and local folder suggestions during tab-completion.
- **Flag Autocompletion**: When the input begins with `-` or `--`, the completer suggests all available flags for the active command (e.g. `--asc`, `--desc`, `--time`, `--size`, `--saveto`).

---

### 5.3 Status Indicators, Badges & Contextual Error Hierarchy

#### Standardized Visual Badges
```text
[ACTIVE]   --> Green Background `#10B981` (Online session)
[OFFLINE]  --> Slate Background `#64748B` (Logged out)
[SVIP]     --> Gold Background  `#F59E0B` (Super VIP account)
[SYNCING]  --> Cyan Background  `#06B6D4` (Background task active)
[QUEUED]   --> Amber Background `#F59E0B` (Offline download pending)
[ERROR]    --> Red Background    `#EF4444` (Operation failed)
```

#### Tri-Part Contextual Error Model
Adopting the best-in-class error formatting from GitHub CLI (`gh`) and the Rust compiler (`rustc`):

```text
✖ Error: Failed to resolve rapid-upload checksum match
  Reason: Remote server returned HTTP 403 Forbidden (BDUSS token has expired)
  Hint:   Run 'login' to re-authenticate or verify your cookies in pcs_config.json
```

```text
✖ Error: Path does not exist on remote cloud storage
  Reason: Directory '/我的资源/Movies/2026' was not found
  Hint:   Run 'ls /我的资源/Movies' to inspect existing folders or check spelling
```

---

### 5.4 Modern Output & Table Formatting

#### The Existing Defect:
In `pcstable/pcstable.go`, tables are rendered without borders or column wrapping. On standard 80-column terminals, output for `ls -l` or `loglist` wraps unpredictably across multiple physical lines.

#### Redesigned Table Specifications:
1. **Rounded Unicode Card Frame (No Inner Vertical Bars)**: Outer card framing with `╭───╮`, `│───│` header rule, whitespace-padded columns (`BorderColumn(false)`), and `╰───╯`.
2. **Header Styling**: Primary accent bold text above a hairline horizontal rule.
3. **Auto-Clamping Terminal Width**: The renderer queries terminal width via `term.GetSize()` and truncates long paths or names with an ellipsis (`…`, or `...` in `--ascii` mode) so the table never wraps.
4. **CJK Double-Width Safety & Ambiguous Width Guidance**: Incorporates `mattn/go-runewidth` to correctly calculate double-column display widths for Chinese characters. For CJK terminal configurations where ambiguous width equals 2 (`runewidth.DefaultCondition.EastAsianWidth = true`), Candidates 1 and 2 and Mode E (pure ASCII) are recommended; the renderer locks `EastAsianWidth = false` for Unicode border primitives or falls back to Mode E (pure ASCII borders: `+`, `-`, `|`) to ensure strict 80-column alignment without line-wrapping.

##### Modern Interactive Table Preview (`ls -l`):
```text
  ╭─────────────────────────────────────────────────────────────────────────╮
  │ #    FS ID          Size   ModTime             File / Directory         │
  │─────────────────────────────────────────────────────────────────────────│
  │ 1    91823019          -   2026-11-14 18:20    Documents/               │
  │ 2    91823024          -   2026-11-15 09:12    Photos/                  │
  │ 3    82910481    1.42 GB   2026-11-15 11:45    ArchLinux-x86.iso        │
  │ 4    77291038   48.20 MB   2026-11-13 22:04    Presentation.pdf         │
  ╰─────────────────────────────────────────────────────────────────────────╯
  Showing 4 items (Total: 1.47 GB)  ·  Workdir: /我的资源
```

##### Dual Mode Support:
- When `--json` is supplied:
  ```json
  [
    {"fs_id": 91823019, "is_dir": true,  "size": 0,          "name": "Documents", "mod_time": 1794766800},
    {"fs_id": 82910481, "is_dir": false, "size": 1524695040, "name": "ArchLinux-x86.iso", "mod_time": 1794829500}
  ]
  ```
- When `--plain` is supplied:
  ```text
  91823019	dir	0	2026-11-14 18:20	Documents/
  82910481	file	1524695040	2026-11-15 11:45	ArchLinux-x86.iso
  ```

---

### 5.5 Modernized Progress Bar Engine

#### Redesigned Single-Task Progress Display:
Replaces `\r[...] ............` with a structured, high-contrast block progress bar:

```text
[1] 78.4% [████████████████████░░░░░░] 1.25 GB / 1.60 GB | 18.4 MB/s | ETA: 19s
```

#### Redesigned Multi-Task Concurrent Download Dashboard:
When multiple files download concurrently (`load > 1`), instead of printing scrolling newline spam (`...\n`), the engine reserves a fixed multi-line viewport using ANSI cursor positioning (`\033[A`):

```text
╭─ Active Multi-Thread Download Pipeline (3 Tasks) ────────────────────────────╮
│ [1] ArchLinux.iso  [██████████████░░░░░░] 68.2% · 18.2 MB/s · ETA: 24s       │
│ [2] Dataset.tar.gz [██████████████████░░] 84.1% · 24.1 MB/s · ETA: 08s       │
│ [3] Backup.zip     [████░░░░░░░░░░░░░░░░] 21.0% ·  8.4 MB/s · ETA: 02m 14s   │
╰────────────────────────────────────────── Total: 50.7 MB/s ──────────────────╯
```

---

## 6. Modern Go TUI/CLI Library Feasibility & Coexistence Architecture

### 6.1 Library Capability & Compatibility Evaluation Matrix

```
┌────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                       GO CLI & TUI LIBRARY EVALUATION MATRIX                                                   │
├──────────────────────┬─────────────┬─────────────┬─────────────────────────────┬───────────────────────────────┬───────────────┤
│ Library              │ Module Path │ Architecture│ Strengths                   │ Constraints                   │ Fit / Action  │
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `charmbracelet/`     │ Go module   │ Pure string │ • Zero background routines  │ • No event loop / input       │ **TIER 1**:   │
│ `lipgloss`           │ pure Go     │ layout engine│ • Declarative styling      │   handling (styling only)     │ Drop-in engine│
│                      │             │             │ • TrueColor & ANSI fallback │                               │ for banners,  │
│                      │             │             │ • Flexible `lipgloss/table` │                               │ badges, tables│
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `charmbracelet/`     │ Go module   │ The Elm     │ • Interactive full-screen   │ • Monopolizes terminal event  │ **TIER 2**:   │
│ `bubbletea`          │ pure Go     │ Architecture│   cloud explorer (`ui`)     │   loop if running continually │ Fullscreen TUI│
│                      │             │ (M-V-U)     │ • Rich widget ecosystem     │ • Must pause `liner` in REPL  │ (`pant ui` /  │
│                      │             │             │   (`bubbles`, AltScreen)    │                               │  `browse`)    │
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `pterm/pterm`        │ Go module   │ Imperative  │ • All-in-one CLI toolkit    │ • Monolithic styling          │ **REJECTED**: │
│                      │ pure Go     │ component   │ • Out-of-the-box spinners   │ • Overlaps existing packages  │ Less compos-  │
│                      │             │ printer     │   and multi-progress bars   │ • Heavyweight dependency tree │ able than Lip │
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `peterh/liner`       │ Existing    │ Line editor │ • Proven Windows & Unix     │ • Rejects ANSI in prompt;     │ **RETAIN**:   │
│ (v1.2.2)             │ dependency  │             │   cross-platform stability  │   solved by Two-Line Prompt   │ Retain for    │
│                      │             │             │ • History & tab autocompleter│                              │ REPL input    │
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `urfave/cli`         │ Existing    │ Router &    │ • Deeply integrated across  │ • Legacy v1 help templates    │ **RETAIN**:   │
│ (v1.22.5)            │ dependency  │ flag parser │   all `cmd_*.go` files      │   need Lipgloss styling       │ Retain command│
│                      │             │             │ • Category command dispatch │                               │ routing layer │
├──────────────────────┼─────────────┼─────────────┼─────────────────────────────┼───────────────────────────────┼───────────────┤
│ `fatih/color`        │ Former      │ ANSI escape │ • Lightweight color wrapper │ • No layout engine (no borders│ **REMOVED**:  │
│ (v1.18.0)            │ dependency  │ printer     │ • Legacy v3/v4.0 usage      │   padding, or auto-wrapping)  │ Purged from   │
│                      │             │             │                             │ • No TrueColor gradients      │ `go.mod`      │
└──────────────────────┴─────────────┴─────────────┴─────────────────────────────┴───────────────────────────────┴───────────────┘
```

---

### 6.2 Decoupled UI Presentation Layer Architecture (`internal/ui`)

To prevent tight coupling between core business logic and terminal rendering, all visual presentation is encapsulated in a dedicated `internal/ui` package:

```
                            ┌──────────────────────────────────────────────┐
                            │               User Invocation                │
                            │         (CLI Argv or Interactive REPL)       │
                            └──────────────────────┬───────────────────────┘
                                                   │
                                                   ▼
                            ┌──────────────────────────────────────────────┐
                            │          internal/cliapp Command Router      │
                            │         (urfave/cli + pcsliner engine)       │
                            └──────────────────────┬───────────────────────┘
                                                   │
                                                   ▼
                            ┌──────────────────────────────────────────────┐
                            │       internal/ui (Unified Styling Layer)    │
                            ├──────────────────────┬───────────────────────┤
                            │ • theme.go           │ • banner.go / logo.go │
                            │ • prompt.go          │ • table.go / output.go│
                            │ • badge.go / help.go │ • notify.go           │
                            │ • progress.go        │ • dashboard.go        │
                            │ • filetype.go        │                       │
                            └──────────────────────┴───────────────────────┘
                                                   │
                              ┌────────────────────┴───────────────────┐
                              ▼                                        ▼
                     [Interactive TTY]                       [Piped / Script / CI]
                   Rich ANSI / Lipgloss / UTF-8            Clean JSON / TSV / No-Color
```

#### Package Responsibilities:
- `theme.go`: Detects and honors TrueColor, ANSI 256, ANSI 16, `FORCE_COLOR` / `CLICOLOR_FORCE`, `NO_COLOR`, and pure 7-bit ASCII profiles.
- `logo.go` & `banner.go`: Renders pan-tools geometric vault monogram (`╭──◈──╮` / `+--*--+`) and responsive startup banners (`>=112` Wide / `74-111` Standard / `<74` Compact).
- `prompt.go`: Renders Starship-style two-line prompt — Line 1 left-aligned user/VIP pill/`~/workdir` + right-aligned quota percentage & usage (`54.2% · 1.1TB/2.0TB`), and Line 2 liner-safe input symbol (`❯ ` or `> `).
- `table.go` & `output.go`: Wraps `lipgloss/table` with terminal width clamping, strict 7-bit ASCII truncation, and structured JSON/TSV writers.
- `filetype.go`: Classifies files (directories, archives, media, code, docs) and applies Nerd-Font-free semantic One Dark colors and size tier styling.
- `badge.go` & `help.go`: Formats high-contrast status pills (`[当前]`, `SVIP 10`) and styled `urfave/cli` help templates.
- `notify.go`: Emits structured tri-part error messages (`错误`, `原因`, `建议`) and quiet-aware status lines.
- `progress.go` & `dashboard.go`: Renders smooth single-task transfer bars and an in-place multi-task `TransferDashboard` for concurrent uploads/downloads (`load > 1`).

---

### 6.3 Modal Execution Pattern for Bubble Tea Coexistence

#### The Conflict:
`peterh/liner` sets the terminal in raw mode using `tmode.ApplyMode()` to capture keystrokes. `charmbracelet/bubbletea` also requests raw mode and alternate screen buffer control (`tea.WithAltScreen()`). Running Bubble Tea concurrently with an active Liner prompt causes terminal deadlocks and corrupted character echoes.

#### The Modal Architectural Pattern:
Bubble Tea is invoked **modally** on demand for the fullscreen interactive cloud explorer (`pant ui` / `pant tui` / `pant browse` in `internal/tui`), while concurrent multi-file uploads/downloads render via the in-place live `TransferDashboard` (`internal/ui/dashboard.go`). When `ui` (`browse`) is invoked from the interactive REPL, `internal/cliapp/repl.go` pauses Liner before dispatching the subcommand and resumes it immediately afterward (using `Pause()` and `Resume()` provided by `*pcsliner.PCSLiner` in `pcsliner/pcsliner.go` to save and restore terminal modes):

```go
// Modal Bubble Tea Execution Pattern (internal/cliapp/repl.go + internal/tui/backend.go)
func runInteractiveCloudExplorer(line *pcsliner.PCSLiner, startDir string) error {
    // Step 1: Pause Liner and restore original terminal mode before Bubble Tea AltScreen
    if line != nil {
        line.Pause()
        defer line.Resume()
    }

    // Step 2: Initialize fullscreen Bubble Tea cloud explorer with Alternate Screen Buffer
    backend := tui.NewPCSBackend()
    model := tui.NewModel(backend, startDir)
    p := tea.NewProgram(model, tea.WithAltScreen())

    // Step 3: Run event loop (blocking until user presses 'q' or 'ctrl+c')
    finalModel, err := p.Run()
    if err != nil {
        return fmt.Errorf("cloud explorer execution failed: %w", err)
    }

    // Step 4: Sync final browsed directory back to active workdir; defer resumes Liner cleanly
    if fm, ok := finalModel.(*tui.Model); ok {
        _ = fm
    }
    return nil
}
```

---

### 6.4 Zero-Overhead Lipgloss Declarative Styling

Unlike UI frameworks that run background timers, goroutines, or event loops, `charmbracelet/lipgloss` is a **pure string transformation library**. Calling `style.Render("text")` performs synchronous in-memory string formatting and returns immediately.
- **Zero Background Goroutines**: Leaves 100% of CPU and network bandwidth available for multi-threaded chunk downloads.
- **Zero Cgo Dependencies**: Fully compatible with Go cross-compilation across all target architectures (`linux/amd64`, `linux/arm64`, `windows/amd64`, `darwin/arm64`).

---

## 7. Migration Plan, Backward Compatibility & Implementation Roadmap

### 7.1 Four-Phase Implementation Roadmap

```
┌───────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                 FOUR-PHASE IMPLEMENTATION ROADMAP                                 │
├─────────┬────────────────────────────┬────────────────────────────────────────────────────────────┤
│ Phase   │ Focus Area                 │ Key Deliverables & Scope                                   │
├─────────┼────────────────────────────┼────────────────────────────────────────────────────────────┤
│ Phase 1 │ UI Layer Decoupling        │ • Create `internal/ui` package                             │
│         │ & Design Tokens            │ • Implement `theme.go` with TrueColor/NO_COLOR/ASCII       │
│         │                            │ • Add badge and notification formatters                    │
├─────────┼────────────────────────────┼────────────────────────────────────────────────────────────┤
│ Phase 2 │ REPL & Banner              │ • Replace `printWelcomeBanner` with `banner.go`            │
│         │ Modernization              │ • Implement Starship-style right-aligned two-line prompt   │
│         │                            │ • Integrate pan-tools vault monogram & responsive banners  │
├─────────┼────────────────────────────┼────────────────────────────────────────────────────────────┤
│ Phase 3 │ Table & Progress Bar       │ • Refactor `pcstable` to use `lipgloss/table`              │
│         │ Modernization              │ • Implement auto-width terminal clamping                   │
│         │                            │ • Upgrade progress bars & live multi-task TransferDashboard│
├─────────┼────────────────────────────┼────────────────────────────────────────────────────────────┤
│ Phase 4 │ Automation & Fullscreen    │ • Implement `--json` and `--plain` output flags            │
│         │ Cloud Explorer TUI         │ • Fullscreen Bubble Tea explorer (`pant ui` / `browse`)    │
│         │                            │ • Full cross-platform validation (Linux/macOS/Windows)     │
└─────────┴────────────────────────────┴────────────────────────────────────────────────────────────┘
```

---

### 7.2 Backward Compatibility Guarantees

The proposed redesign strictly enforces backward compatibility to ensure that existing user workflows and scripts continue operating seamlessly:

1. **Command Aliases Preservation**:
   All historic single-character aliases (`d` for download, `u` for upload, `l` / `ll` for ls, `s` for search, `t` for tree) remain 100% functional.
2. **Configuration File Invariance**:
   The configuration schema (`~/.config/pan-tools/pcs_config.json`, configurable via `PANT_CONFIG_DIR`) remains completely unmodified. User credentials (BDUSS, PTOKEN), download directory settings, and network speed limits are loaded without migration scripts.
3. **Piped Script Safety**:
   When stdout is redirected to a pipeline or non-TTY file, all banners, progress bars, and ANSI escape sequences are automatically suppressed, preventing regression in automated backup scripts.
4. **Cross-Platform Terminal Support**:
   Windows 10/11 Terminal, classic Windows `cmd.exe` (via `muesli/termenv` & `charmbracelet/lipgloss`), Linux X11/Wayland terminals, and macOS Terminal.app are natively supported without requiring external terminal multiplexers.

---

### 7.3 Verification & Validation Protocols

Before publishing production releases of the redesigned CLI, the following verification suite must be executed:

1. **Codebase Compilation & Test Suite**:
   ```bash
   go build -o pant .
   go test ./...
   ```
   *Criteria*: All packages compile cleanly with exit code 0; zero test failures.

2. **Repository Hygiene Verification**:
   ```bash
   git status --porcelain
   ```
   *Criteria*: No unexpected modifications to core protocol or networking packages.

3. **Terminal Width Alignment & ASCII Art Verification**:
   ```bash
   # Run in standard 80-column terminal simulation
   COLUMNS=80 ./pant
   ```
   *Criteria*: Logo renders cleanly; box borders align with exact 76-column margins; zero line wraps or jagged edges.

4. **Liner ANSI Safety Verification**:
   Verify that the Starship-style two-line prompt emits Line 1 directly to stdout and passes only printable runes in Line 2, confirming that `line.Prompt()` never returns `ErrInvalidPrompt`.

5. **Machine Output Pipe Verification**:
   ```bash
   ./pant ls --plain | awk '{print $5}'
   ./pant ls --json | jq '.[].name'
   ```
   *Criteria*: Emits valid, parseable stream data without decorative borders or ANSI color codes.

---
*End of Design Proposal.*
