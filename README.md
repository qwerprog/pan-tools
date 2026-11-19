# pan-tools (`pant`)

现代百度网盘 Go 命令行与交互式终端客户端（可执行命令：`pant`）。

## 核心特性

- **全屏交互式网盘浏览器（`pant ui` / `tui` / `browse`）**：基于 Bubble Tea 的双栏文件管理器，支持方向键 / `jkhl` 浏览、`/` 实时模糊过滤、`Space` 多选、新建目录、重命名、回收站删除、直接上传/下载，退出后自动同步当前网盘工作目录。
- **终端扫码登录与交互式登录菜单（`pant login` / `pant login -qr`）**：零依赖支持百度网盘 App / 百度 App 终端直接扫码登录，自动获取 `BDUSS` 与网盘专属 `STOKEN`（同步生成 `0600` 权限临时 `qrcode.png` 并在结束后自动清理）；同时支持粘贴 `Cookies` 或输入 `BDUSS + STOKEN` 登录。
- **现代终端视觉与双行提示符**：内置 Atom One Dark 主题、自适应启动面板、圆角表格与多任务并发实时传输看板，支持 `starship`（默认）与 `boxed` 双行提示符切换，以及 `--no-color` / `NO_COLOR`、`--ascii` / `PANT_ASCII` 降级。
- **脚本与自动化友好**：支持 `--quiet`（`-q`）、`--json`（结构化 JSON）与 `--plain`（无边框 TSV）输出，严格分离 `stdout` 数据与 `stderr` 错误提示。
- **高速传输内核**：支持大目录翻页、多线程分片并发下载、秒传校验、最高 128GB 单文件分片上传、断点续传、分享口令/链接转存、离线下载与回收站管理。

---

## 快速开始

### 1. 获取与编译

可从 [GitHub Releases](https://github.com/qwerprog/pan-tools/releases) 下载对应平台预编译程序，或自行编译（需 Go 1.25+）：

```bash
# 本机编译（Windows 下输出为 pant.exe）
go build -o pant .

# 交叉编译示例（编译 Windows 64 位）
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o pant.exe .
```

### 2. 登录百度账号

```bash
# 打开交互式登录菜单（默认推荐扫码登录）
pant login

# 直接使用百度网盘 App / 百度 App 扫码登录
pant login -qr

# 或使用浏览器 Cookies / BDUSS + STOKEN 登录
pant login -cookies="BDUSS=xxx; STOKEN=yyy; ..."
pant login -bduss=<BDUSS> -stoken=<STOKEN>
```

### 3. 常用操作

```bash
pant                 # 进入交互式 REPL 模式（支持 Tab 补全与双行提示符）
pant ui              # 启动全屏交互式网盘浏览器
pant ls /            # 列出根目录
pant cd /我的资源     # 切换工作目录
pant d 1.mp4         # 下载文件（也支持目录）
pant u ./a.zip /备份  # 上传文件或目录
pant tf <分享链接>    # 转存分享链接或整段分享口令
```

---

## 常用命令速查

直接运行 `pant` 进入 REPL 后，命令可省略 `pant` 前缀；任意命令附加 `-h` 可查看完整参数说明。

| 类别 | 命令（别名） | 说明 |
| :--- | :--- | :--- |
| **浏览与管理** | `ui` (`tui`, `browse`) | 全屏交互式网盘文件管理器 |
| | `ls` (`l`, `ll`), `cd`, `pwd`, `tree` | 目录查看、切换与树形展示 |
| | `meta`, `search` (`s`), `mkdir`, `rm`, `cp`, `mv`, `rename` (`rn`) | 文件元信息、搜索、建目录、删改移动 |
| **上传下载** | `download` (`d`), `upload` (`u`), `locate` | 并发分片下载、秒传/分片上传、获取直链 |
| **分享与云端** | `transfer` (`tf`), `share` | 分享链接/口令转存、创建/管理分享 |
| | `offlinedl` (`od`, `clouddl`), `recycle` | 离线下载任务、回收站查看/还原/清理 |
| **账号与系统** | `login`, `who`, `su`, `loglist`, `logout`, `quota` | 多账号管理与网盘容量查询 |
| | `config`, `env`, `update`, `tool` | 配置查看/修改、环境变量、自更新、加解密/校验工具 |

---

## 配置与环境变量

- **默认配置目录**：`~/.config/pan-tools`（Windows 为 `%APPDATA%\pan-tools`；若程序同目录已有 `pcs_config.json` 则优先使用同目录）。
- **常用环境变量**（可通过 `pant env` 查看）：
  - `PANT_CONFIG_DIR`：自定义配置文件目录
  - `PANT_PROMPT_STYLE`：REPL 提示符风格（`starship` / `boxed`，也可通过 `pant config set -prompt_style boxed` 持久化）
  - `PANT_ASCII`：强制纯 7-bit ASCII 符号输出（等同 `--ascii`）
  - `PANT_VERBOSE`：启用调试日志（等同 `--verbose`）
  - `PANT_UPDATE_REPO`：覆盖 `pant update` 检查的 GitHub 仓库（默认 `qwerprog/pan-tools`）
  - `NO_COLOR` / `CLICOLOR` / `CLICOLOR_FORCE`：控制终端彩色输出
- **并发与限速建议**：
  - 普通用户建议保持 `max_parallel=1`、`max_download_load=1`，避免过高并发触发百度限速。
  - SVIP 用户可适当设置 `max_parallel=10~20`、`max_download_load=1~2`（通过 `pant config set` 修改）。

---

## 文档与反馈

- 架构与接口说明：[docs/README.md](docs/README.md)
- CLI / TUI 设计规范：[CLI_REDESIGN_PROPOSAL.md](CLI_REDESIGN_PROPOSAL.md)
- 问题反馈：[qwerprog/pan-tools Issues](https://github.com/qwerprog/pan-tools/issues)

---

## 开源协议与致谢

本项目采用 **Apache License 2.0** 开源（详见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)），基于以下上游项目派生并深度重构：

- `iikira/BaiduPCS-Go`（Copyright 2016-2020 iikira）
- [qjfoidnh/BaiduPCS-Go](https://github.com/qjfoidnh/BaiduPCS-Go)（Copyright 2018-2026 qjfoidnh）

在保留高速分片传输与网盘协议内核的基础上，`pan-tools`（`pant`）完成了模块化重构、现代终端 UI、Bubble Tea 全屏文件管理器与终端扫码登录，并移除了 GPL-3.0 外部依赖（以纯 Go 标准库与 HTTPS 网盘接口实现替代）。
