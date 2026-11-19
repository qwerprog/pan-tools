# pan-tools (`pant`) — 现代百度网盘命令行与交互式终端客户端

仿 Linux shell 文件处理命令的现代百度网盘命令行与交互式终端客户端（`pant`）。

## pan-tools 现代终端特性概览

- **全屏交互式网盘文件管理器（`pant ui` / `tui` / `browse`）**：基于 Bubble Tea 的全屏双栏网盘浏览器，支持方向键/`jkhl` 漫游、`/` 实时模糊过滤、`Space` 多选、排序切换、新建目录、重命名、回收站删除、直接上传/下载，退出后自动同步当前工作目录。
- **终端扫码登录与交互式登录菜单（`pant login` / `pant login -qr`）**：零依赖支持百度网盘 App / 百度 App 终端直接扫码登录（自动获取 `BDUSS` 与网盘专属 `STOKEN`，深浅色终端自适应 `Dense1x2` 二维码渲染并同步生成 `0600` 临时 `qrcode.png` 自动清理），同时支持交互式选择粘贴 `Cookies` 或输入 `BDUSS + STOKEN` 登录。
- **Atom One Dark 终端视觉与无障碍降级**：自适应终端宽度的响应式启动面板、圆角 Unicode 表格、可选两套双行提示符风格（默认 `starship` 极简右对齐 `用户名  SVIP  ~/路径` + `❯ `，或 `boxed` 圆角框线 `╭─ pan-tools ▏ ● 用户名 ...` + `╰─❯ `，支持 `pant config set -prompt_style boxed` / `PANT_PROMPT_STYLE` / `--prompt-style` 切换，UID 默认部分脱敏）以及多任务并发实时传输看板（`TransferDashboard`），全局支持 `--no-color` / `NO_COLOR` 与 `--ascii` / `PANT_ASCII` 严格 7-bit ASCII 降级。
- **脚本与 AI 自动化友好**：全局支持 `--quiet` / `-q`（静默隐藏横幅、进度条与装饰提示）、`--no-color`、`--ascii`、`--verbose`；数据查询子命令（`ls`、`search`、`who`、`quota`、`loglist`、`share list`、`recycle list`、`offlinedl list`）支持 `--json`（结构化 JSON）与 `--plain`（无边框 TSV），严格分离 `stdout` 数据与 `stderr` 结构化错误及排错建议。
- **配置与命令体系**：默认使用 `PANT_CONFIG_DIR` / `~/.config/pan-tools`（Windows 下为 `%APPDATA%\pan-tools`），完整支持全套网盘子命令、快捷别名与 `tool enc`/`dec`（默认密钥 `"pan-tools"`）。
- **构建命令**：`go build -o pant .`（详细分层架构见 [docs/README.md](docs/README.md)，UI/CLI 设计规范见 [CLI_REDESIGN_PROPOSAL.md](CLI_REDESIGN_PROPOSAL.md)）。

## 注意

当前版本支持分享口令/链接转存、大目录翻页与最高 128GB 单文件上传。

本软件不提供超出官方客户端的下载提速, 普通用户和SVIP的配置建议参见 [显示和修改程序配置项](#显示和修改程序配置项)

<!-- toc -->
## 目录

- [特色](#特色)
- [版本更新](#版本更新)
- [编译/交叉编译 说明](#编译交叉编译-说明)
- [下载/运行 说明](#下载运行-说明)
  * [安装](#安装)
  * [Windows](#windows)
  * [Linux / macOS](#linux--macos)
  * [Android / iOS](#android--ios)
- [命令列表及说明](#命令列表及说明)
  * [注意 ! ! !](#注意---)
  * [全屏交互式网盘浏览器](#全屏交互式网盘浏览器)
  * [检测程序更新](#检测程序更新)
  * [登录百度帐号](#登录百度帐号)
  * [列出帐号列表](#列出帐号列表)
  * [获取当前帐号](#获取当前帐号)
  * [切换百度帐号](#切换百度帐号)
  * [退出百度帐号](#退出百度帐号)
  * [获取网盘配额](#获取网盘配额)
  * [切换工作目录](#切换工作目录)
  * [输出工作目录](#输出工作目录)
  * [列出目录](#列出目录)
  * [列出目录树形图](#列出目录树形图)
  * [获取文件/目录的元信息](#获取文件目录的元信息)
  * [搜索文件](#搜索文件)
  * [下载文件/目录](#下载文件目录)
  * [上传文件/目录](#上传文件目录)
  * [获取下载直链](#获取下载直链)
  * [创建目录](#创建目录)
  * [删除文件/目录](#删除文件目录)
  * [拷贝文件/目录](#拷贝文件目录)
  * [移动/重命名文件/目录](#移动重命名文件目录)
  * [重命名单个文件/目录](#重命名单个文件目录)
  * [转存文件/目录](#转存文件目录)
  * [分享文件/目录](#分享文件目录)
    + [设置分享文件/目录](#设置分享文件目录)
    + [列出已分享文件/目录](#列出已分享文件目录)
    + [取消分享文件/目录](#取消分享文件目录)
  * [离线下载](#离线下载)
    + [添加离线下载任务](#添加离线下载任务)
    + [精确查询离线下载任务](#精确查询离线下载任务)
    + [查询离线下载任务列表](#查询离线下载任务列表)
    + [取消离线下载任务](#取消离线下载任务)
    + [删除离线下载任务](#删除离线下载任务)
  * [回收站](#回收站)
    + [列出回收站文件列表](#列出回收站文件列表)
    + [还原回收站文件或目录](#还原回收站文件或目录)
    + [删除回收站文件或目录/清空回收站](#删除回收站文件或目录清空回收站)
  * [显示程序环境变量](#显示程序环境变量)
  * [显示和修改程序配置项](#显示和修改程序配置项)
  * [测试通配符](#测试通配符)
  * [工具箱](#工具箱)
- [初级使用教程](#初级使用教程)
  * [1. 查看程序使用说明](#1-查看程序使用说明)
  * [2. 登录百度帐号 (必做)](#2-登录百度帐号-必做)
  * [3. 切换网盘工作目录](#3-切换网盘工作目录)
  * [4. 网盘内列出文件和目录](#4-网盘内列出文件和目录)
  * [5. 下载文件](#5-下载文件)
  * [6. 设置下载最大并发量](#6-设置下载最大并发量)
  * [7. 恢复默认配置](#7-恢复默认配置)
  * [8. 退出程序](#8-退出程序)
- [已知问题](#已知问题)
- [TODO](#todo)
- [交流反馈](#交流反馈)

<!-- tocstop -->

# 特色

多平台支持, 支持 Windows, macOS, linux, 移动设备等.

百度帐号多用户支持;

通配符匹配网盘路径和 Tab 自动补齐命令和路径, [通配符_百度百科](https://baike.baidu.com/item/通配符);

[下载](#下载文件目录)网盘内文件, 支持多个文件或目录并发下载、实时 `TransferDashboard` 看板、断点续传和单文件并行分片下载;

[上传](#上传文件目录)本地文件, 支持上传最大 128GB 大文件、多文件并发上传、秒传检测、分片断点续传及 `Ctrl+C` 优雅保存进度;

[全屏浏览器](#全屏交互式网盘浏览器)（`pant ui` / `tui` / `browse`）, 零学习成本全键盘浏览、过滤、多选与传输;

[转存](#转存文件目录)其他用户分享的文件, 支持公开免密链接、带密码分享链接及整段百度网盘分享口令文本;

[离线下载](#离线下载), 支持http/https/ftp/电驴/磁力链协议.
# 版本更新

**2026.08.21** v4.0.2
- 恢复分片上传断点续传支持（`Ctrl+C` 优雅落盘未完成分片）
- 修复转存误报STOKEN缺失
- feat: add --fs_id flag to transfer command to support granular file selection
- fix: 支持下载/查看对象数量超过1000的目录
- fix: remove quotes from cookie values to prevent invalid byte error
- 修复上传失败错误码仍为0
- 转存支持分享口令
- 上传链接复用优化


**2026.03.26** v4.0.1
- 紧急修复ls等命令的param error

**2025.10.29** v4.0.0
- 上传重新支持跳过秒传`--norapid`
- 上传同名文件覆盖策略`--policy`支持`skip`,`overwrite`,`rsync`; 支持`config`配置全局默认策略
- 当时因接口变化临时移除上传断点续传（已在 v4.0.2+ 恢复支持）, 下载不受影响
- 增加`config`配置`proxy_hostnames`, 国外VPS用户如遇上传问题可尝试为`pan.baidu.com`配置回国代理
- 其他细节优化

**2025.08.30** v3.9.9
- 最大上传单文件支持至128G
- 上传速度优化
- 下载取消文件预分配
- 因官方接口变动上传文件强制计算秒传
- transfer命令修复`--download`参数

**2025.08.29** v3.9.8
- 全面修复了上传文件的问题
- 全面修复了下载文件的问题
- 去除部分已不可用的功能和已不支持的参数

**2025.01.07** v3.9.7
- fix #359, #360
- fix #339

**2024.12.14** v3.9.6
- 关闭秒传转存功能
- 修复常规转存失败

**2023.09.30** v3.9.5
- 恢复秒传转存功能, 使用前需设置accessToken, 参见setastoken --help
- 本地文件上传用秒传无须accessToken
- fix #301
- fix #302

**2023.09.06** v3.9.5-beta
- 恢复了秒传转存(支持长短链), 感谢油猴脚本开发者tousakarin的贡献
- 新秒传接口需要开发者授权, 稳定性未知, 该测试版本仅供有秒传强需求的用户试用, 请谨慎更新

**2023.09.05** v3.9.4
- fix #244, 修复断点上传时偶发崩溃
- 优化本地上传秒传失败时的处理逻辑

**2023.08.26** v3.9.3
- 因官方接口从原理层面封禁秒传, 取消秒传转存功能
- 更新部分使用说明
- 建议使用文件上传功能的用户更新此版本

**2023.06.03** v3.9.2
- 修复秒传链接无法转存, 因官方接口变动秒传已不再支持短链接格式
- 修复上传文件无法使用秒传
- fix #254 支持-f参数输出带密码分享链接
- fix #251 根据mengzonefire同志提供函数增加md5解密

**2023.03.19** v3.9.1
- 修复秒传转存返回错误码9019

**2022.12.04** v3.9.0:
- 优化转存错误提示
- fix #239
- update go version to 1.18

**2022.11.25** v3.8.9:
- fix #234, 继续修复无法转存文件

**2022.11.12** v3.8.8:
- fix #234, 修复无法转存文件

**2022.2.18** v3.8.7:
- fix #175, 在正式上传前即进行文件大小检测

**2022.2.14** v3.8.6:
- fix #160 #173, 修复上传出现空文件的bug
- fix #165, 支持自带提取码的转存链接
- fix #175, upload增加-policy=rsync策略, 配合--norapid使用, 只跳过大小未发生改变的文件
- 鉴于 #172, 建议下载线程数最大不超过12

**2022.1.1** v3.8.5:
#### 该版本存在已知问题将导致上传文件失败及出现空文件，建议跳过更新
- 2022新年好, 本次更新增加较多特性, 欢迎测试
- fix #146, 提前fail和skip上传策略中重名文件的检测环节（存在问题）
- fix #158, config可配置关闭文件名合法性检测
- fix #141, download增加--mtime选项可保持文件修改时间
- fix #130, config可配置force_login_username, 强制登录指定用户名
- 首条下载链接不可用时自动切换, 增加下载成功率

**2021.10.6** v3.8.4:
- fix 登录时可能出现内存溢出
- 上传文件名允许包含单引号

**2021.8.27** v3.8.3:
- fix 更换默认panUA解决svip限速
- fix 移除失效的秒传修复功能 
- 优化秒传逻辑, 提高成功率
- 优化秒传导出逻辑, 提高新文件的导出成功率  

**2021.7.20** v3.8.2:
- fix 读取大量文件信息容易超时
- fix 秒传链接文件名带"#"时解析错误
- share list增加分享下载数显示  
- config增加配置: 上传的同名文件处理策略

**2021.6.9** v3.8.1:
- fix 部分旧链接无法转存
- 增加上传同名文件自动跳过选项

**2021.5.21** v3.8.0:
- fix 上传到100M左右自动回滚（待测试）
- fix 个别正常的秒传链接无法转存
- fix 文件名含有百分号导出异常
- 优化上传重试策略（待测试）

**2021.4.14** v3.7.9:
- fix 上传时异常退出导致无法加载断点信息
- fix 上传偶发出现0B/s卡住
- 上传时预先检查文件名合法性
- 在线更新使用镜像源加速

**2021.3.20** v3.7.8:

- 优化了上传的输出信息格式
- 优化了上传逻辑，提升上传速度
- transfer增加--fix参数，可转存被屏蔽的秒传链接（inspired by [dupan-rapid-extract](https://github.com/mengzonefire/dupan-rapid-extract)）

**2021.3.11** v3.7.7:

- fix 移动和重命名文件时末尾```/```导致报错
- fix 3.7.2版本后在线升级无效
- fix 转存误报缺少STOKEN

**2021.2.23** v3.7.6:

- fix 下载文件报```x509: certificate is valid```错误
- 完善了下载错误的捕获种类
- download增加--fullpath参数，本地目录保留网盘从根目录开始的完整结构

**2021.2.8** v3.7.5:

- fix 某些时候误报stoken缺失
- fix windows平台上秒传链接转存失败
- fix 某些时候pcs请求缺少Host
- 当分享链接包含多文件/目录时，可选归档到第一个文件命名的目录里（不支持秒传）

**2021.1.31** v3.7.4:

- fix 下载目录会丢失目录结构
- fix 分享列表状态信息显示错误
- 支持自定义文件上传服务器

**2021.1.22** v3.7.3:

- 分享支持自定义分享码和有效天数
- 转存支持转存完毕后自动下载到默认目录
- 增加恢复默认配置功能
- tree命令支持指定输出最大层数和带fsid输出

**2021.1.9** v3.7.2:

- 基本修复了登录验证失效问题([#15](https://github.com/qjfoidnh/BaiduPCS-Go/issues/15))
- 优化下载模块的实现策略, 保证稳定性同时进一步提升下载速度 (需按[显示和修改程序配置项](#显示和修改程序配置项)中建议修改)
- update 功能恢复, 以后可以在线升级了
- 支持导出秒传链接不写文件, 直接输出到控制台; 支持通用秒传格式导出, 具体参见export --help
- 其他bug修正

**2021.1.2** v3.7.1:

- 支持了多文件并发上传，文件并发数和单文件分片数可在配置中指定
- 修复了最大同时下载文件数配置不生效的问题
- 修正了部分显示和帮助的错误

**2020.12.19** v3.7.0:

* 替换了iikira版本的失效仓库
* 转存功能支持旧的短链接
* 默认关闭下载文件校验，配置文件可设置开启
* 修复了关闭校验时会误报下载失败的问题
* 转存功能除了cookies方式登录，现已支持用户名密码登录和bduss登录；bduss登录需同时指定stoken

**2020.11.08** v3.6.3:

* 修复转存失败
* 修复分享文件失败


# 编译/交叉编译 说明

本机编译可直接指定输出可执行文件名 `pant`（Windows 下为 `pant.exe`）：
```
# Linux / macOS
go build -o pant .

# Windows
go build -o pant.exe .
```

交叉编译时设置好 `GOOS` 和 `GOARCH` 环境变量（运行 `go tool dist list` 查看所有支持的 `GOOS/GOARCH`）：

## Linux/Darwin 例子: 编译 Windows 下的 64 位程序
```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o pant.exe .
```
## Windows 例子: 编译 Linux 下的 32 位程序
```
set GOOS=linux
set GOARCH=386
set CGO_ENABLED=0
go build -o pant .
```

# 下载/运行 说明

Go 语言程序，可直接通过源码 `go build -o pant .` 编译，或从 [GitHub Releases](https://github.com/qwerprog/pan-tools/releases) 下载对应平台的可执行文件。

如果程序运行时输出乱码, 请检查下终端的编码方式是否为 `UTF-8`.

使用本程序之前, 建议学习一些 linux 基础知识 和 基础命令.

如果未带任何参数运行程序, 程序将会进入仿 Linux shell 的交互式 REPL 模式, 可直接运行相关命令.

REPL 交互模式提供两套可选双行提示符风格：默认 `starship` 极简右对齐风格（首行左侧展示 `<用户名>  <VIP等级>  ~/工作目录`、右侧对齐展示空间使用率与配额如 `54.2% · 1.1TB/2.0TB`，第二行为输入提示符 `❯ `，`--ascii` 下为 `> `），以及 `boxed` 圆角框线风格（`╭─ pan-tools ▏ ● 用户名 [VIP: ...] [dir: ...] [quota: ...]` + `╰─❯ `，`--ascii` 下为 `+- pan-tools | ...` + `+-> `）；可通过 `pant config set -prompt_style boxed`、`PANT_PROMPT_STYLE` 或 `--prompt-style` 切换。

程序会提供相关命令的使用说明.

## 安装


## Windows

程序应在 命令提示符 (Command Prompt) 或 PowerShell 中运行, 在 mintty (例如: GitBash) 可能会有显示问题.

也可直接双击程序运行, 具体使用方法请参见 [命令列表及说明](#命令列表及说明) 和 [初级使用教程](#初级使用教程).

## Linux / macOS

程序应在 终端 (Terminal) 运行.

具体使用方法请参见 [命令列表及说明](#命令列表及说明) 和 [初级使用教程](#初级使用教程).

## Android / iOS

> Android / iOS 移动设备操作比较麻烦, 不建议在移动设备上使用本程序. 移动设备不可直接使用预编译的Linux arm64版本, 使用者需下载源码自行交叉编译.

安卓, 建议使用 [Termux](https://termux.com) 或 [NeoTerm](https://github.com/NeoTerm/NeoTerm) 或 终端模拟器, 以提供终端环境.

苹果iOS, 需要越狱, 在 Cydia 搜索下载并安装 MobileTerminal, 或者其他提供终端环境的软件.

具体使用方法请参见 [命令列表及说明](#命令列表及说明) 和 [初级使用教程](#初级使用教程).

# 命令列表及说明

## 注意 ! ! !

命令的前缀 `pant` 为指向程序运行的可执行文件名（兼容旧二进制名用法）。

直接运行 `pant` 未带任何子命令参数时，程序进入交互式 REPL 模式；在 REPL 内运行以下命令时，去掉前缀 `pant` 即可！

交互式 REPL 已支持按 `Tab` 键自动补全命令、子命令、本地路径和网盘路径。

## 全屏交互式网盘浏览器
```
pant ui [初始网盘目录]
pant tui [初始网盘目录]
pant browse [初始网盘目录]
```

启动基于 Bubble Tea 的全屏交互式网盘文件管理器：
- `↑/↓` 或 `j/k` 移动光标，`←/→` 或 `h/l`/`Enter`/`Backspace` 进出目录
- `Space` 切换多选，`a` 全选当前列表，`/` 实时模糊过滤，`s` 切换排序，`r` 强制刷新
- `d` 下载选中项，`u` 上传本地文件或目录（支持 `~/...` 展开），`n` 新建目录，`R` 重命名，`x` 移入回收站
- `?` 查看快捷键帮助，`q` 退出并自动将最后浏览目录同步为当前工作目录

## 检测程序更新
```
pant update
```
默认检查 [qwerprog/pan-tools Releases](https://github.com/qwerprog/pan-tools/releases) 的最新版本，也可通过环境变量 `PANT_UPDATE_REPO=<owner/repo>` 指定其他 GitHub 仓库。

## 登录百度帐号

### 交互式选择登录 / 手机扫码登录 (推荐)

在交互式终端直接运行 `pant login` 将打开方向键/数字键选择菜单（默认选中 **扫码登录（推荐，支持百度网盘/百度 App）**，也可选择粘贴 Cookies 或输入 BDUSS + STOKEN 登录）。

也可以通过 `-qr`（或 `--qr` / `--qrcode`）直接进入手机扫码登录，无需手动抓包复制 Cookie，自动获取 `BDUSS` 与百度网盘专属 `STOKEN`（终端直接渲染高对比度二维码，并在配置目录下生成 `0600` 权限临时 `qrcode.png`，登录完成或按 `Ctrl+C` 取消后自动清理）：
```
pant login
pant login -qr
```

### 使用百度 Cookies 来登录百度账号

[关于 获取百度 Cookies](https://jingyan.baidu.com/article/5553fa829a6a9e65a23934b0.html)
教程中为百度经验的Cookies获取, 这里换成百度网盘首页即可.

```
pant login -cookies=<Cookies>
```

### 使用百度 BDUSS 和 百度网盘 STOKEN 来登录百度账号

[关于 获取百度 BDUSS](https://blog.csdn.net/ykiwmy/article/details/103730962) STOKEN 获取方式与 BDUSS 基本相同。注意 STOKEN 必须在百度网盘页面获取，否则无效.
STOKEN是cookie中的一个字段, 注意不是bdstoken, 如果拿到的STOKEN里没有大写字母多半是拿错了

```
pant login -bduss=<BDUSS> -stoken=<STOKEN>
```

#### 例子
```
# 交互式菜单选择登录方式（默认推荐扫码登录）
pant login

# 直接使用百度网盘 App / 百度 App 扫码登录
pant login -qr

# 使用 BDUSS + STOKEN 登录
pant login -bduss=1234567 -stoken=234567

# 使用完整网盘 Cookies 登录
pant login -cookies="BAIDUID=50949C0890YG9735EA6Q3870AFE38:FG=1; BIDUPSID=112335C0ACCAFFJW675EA69A870AFE38; PSTM=1981928511; BDORZ=D6745EBF6F3SW24E515D22A1598; PANWEB=1; BDUSS=ASAYUGFHSTFKGBGSU; STOKEN=gfsdge9gisfgspig34254d7879eee5756b10sgeyrw5vyw342td510ffc9414d32251; SCRC=cwrywec5evyetra26bvvehefvfg6a8; BDCLND=C%4sfgGysrZ%2BML6; PANPSC=wreyewygdfhdggedhsdfg4353"
```

## 列出帐号列表

```
pant loglist
```

列出所有已登录的百度帐号

## 获取当前帐号

```
pant who
```

## 切换百度帐号

切换已登录的百度帐号
```
pant su <uid>
```
```
pant su

请输入要切换帐号的 # 值 >
```

## 退出百度帐号

退出当前登录的百度帐号
```
pant logout
```

程序会进一步确认退出帐号, 防止误操作.

## 获取网盘配额

```
pant quota
```
获取网盘的总储存空间, 和已使用的储存空间

## 切换工作目录
```
pant cd <目录>
```

### 切换工作目录后自动列出工作目录下的文件和目录
```
pant cd -l <目录>
```

#### 例子
```
# 切换 /我的资源 工作目录
pant cd /我的资源

# 切换 上级目录
pant cd ..

# 切换 根目录
pant cd /

# 切换 /我的资源 工作目录, 并自动列出 /我的资源 下的文件和目录
pant cd -l 我的资源

# 使用通配符
pant cd /我的*
```

## 输出工作目录
```
pant pwd
```

## 列出目录

列出当前工作目录的文件和目录或指定目录
```
pant ls
```
```
pant ls <目录>
```

### 可选参数
```
-l:       详细显示（包含 fs_id、app_id、创建日期与 md5）
--asc:    升序排序（兼容 -asc）
--desc:   降序排序（兼容 -desc）
--time:   根据时间排序（兼容 -time）
--name:   根据文件名排序（兼容 -name）
--size:   根据大小排序（兼容 -size）
--json:   以结构化 JSON 格式输出结果（适配管道与脚本）
--plain:  以纯制表符分隔（TSV）格式输出，无边框和颜色
```

#### 例子
```
# 列出 我的资源 内的文件和目录
pant ls 我的资源

# 绝对路径
pant ls /我的资源

# 降序排序
pant ls -desc 我的资源

# 按文件大小降序排序
pant ls -size -desc 我的资源

# 使用通配符
pant ls /我的*
```

## 列出目录树形图

列出当前工作目录的文件和目录或指定目录的树形图
```
pant tree <目录>

# 默认获取工作目录元信息
pant tree
```

## 获取文件/目录的元信息
```
pant meta <文件/目录1> <文件/目录2> <文件/目录3> ...

# 默认获取工作目录元信息
pant meta
```

#### 例子
```
pant meta 我的资源
pant meta /
```

## 搜索文件

按文件名搜索文件（不支持查找目录）。

默认在当前工作目录搜索.

```
pant search [-path=<需要检索的目录>] [-r] <关键字>
```

#### 例子
```
# 搜索根目录的文件
pant search -path=/ 关键字

# 搜索当前工作目录的文件
pant search 关键字

# 递归搜索当前工作目录的文件
pant search -r 关键字
```

## 下载文件/目录
```
pant download <网盘文件或目录的路径1> <文件或目录2> <文件或目录3> ...
pant d <网盘文件或目录的路径1> <文件或目录2> <文件或目录3> ...
```

### 可选参数
```
  --test          测试下载, 此操作不会保存文件到本地
  --ow            overwrite, 覆盖已存在的文件
  --status        输出所有线程的工作状态
  --save          将下载的文件直接保存到当前工作目录
  --saveto value  将下载的文件直接保存到指定的目录
  -x              为文件加上执行权限, (windows系统无效)
  --mode value    下载模式, 可选值: pcs, stream, locate, 默认为 locate, 相关说明见上面的帮助 (default: "locate")
  -p value        指定下载线程数 (default: 0)
  -l value        指定同时进行下载文件的数量 (default: 0)
  --retry value   下载失败最大重试次数 (default: 3)
  --nocheck       下载文件完成后不校验文件

```

下载的文件默认保存到用户主目录下的 **Downloads** 目录（Linux/macOS 为 `$HOME/Downloads`，Windows 为程序所在目录的 `Downloads` 目录），重名的本地文件默认自动跳过！

通过 `pant config set -savedir <savedir>`, 自定义保存的目录.

多文件并发下载（`-l > 1`）时自动启用底部原地刷新的 `TransferDashboard` 实时看板，避免进度滚屏。

支持多个文件或目录下载，自动跳过已存在的同名本地文件（可通过 `--ow` 强制覆盖）！


#### 例子
```
# 设置保存目录, 保存到 D:\Downloads
# 注意区别反斜杠 "\" 和 斜杠 "/" !!!
pant config set -savedir D:/Downloads

# 下载 /我的资源/1.mp4
pant d /我的资源/1.mp4

# 下载 /我的资源 整个目录!!
pant d /我的资源

# 下载网盘内的全部文件!!
pant d /
pant d *
```

## 上传文件/目录
```
pant upload <本地文件/目录的路径1> <文件/目录2> <文件/目录3> ... <目标目录>
pant u <本地文件/目录的路径1> <文件/目录2> <文件/目录3> ... <目标目录>
```

* 上传默认采用分片并发上传的方式，上传的文件将会保存到 `<目标目录>`，支持分片断点续传（首次按 `Ctrl+C` 等待当前进行中的分片落盘保存断点后优雅退出，多文件并发上传时自动启用 `TransferDashboard` 实时看板）。

* 遇到同名文件会自动跳过, 也可配置`upload_policy`选择覆盖或者只跳过同大小文件

* 当上传的文件名和网盘的目录名称相同时, 不会覆盖目录, 防止丢失数据.

* 所有上传均默认检测秒传, 可添加参数`--norapid`跳过


#### 例子:
```
# 将本地的 C:\Users\Administrator\Desktop\1.mp4 上传到网盘 /视频 目录
# 注意区别反斜杠 "\" 和 斜杠 "/" !!!
pant upload C:/Users/Administrator/Desktop/1.mp4 /视频

# 将本地的 C:\Users\Administrator\Desktop\1.mp4 上传到网盘 /视频 目录, 不检测秒传
pant upload C:/Users/Administrator/Desktop/1.mp4 /视频 --norapid

# 将本地的 C:\Users\Administrator\Desktop\1.mp4 和 C:\Users\Administrator\Desktop\2.mp4 上传到网盘 /视频 目录
pant upload C:/Users/Administrator/Desktop/1.mp4 C:/Users/Administrator/Desktop/2.mp4 /视频

# 将本地的 C:\Users\Administrator\Desktop 整个目录上传到网盘 /视频 目录, 只覆盖与本地大小不同的同名文件
pant upload C:/Users/Administrator/Desktop /视频 --policy rsync
```

## 获取下载直链
```
pant locate <文件1> <文件2> ...
```

#### 例子:

```
pant config set -user_agent "netdisk;2.2.51.6;netdisk;10.0.63;PC;android-android"
```

## 创建目录
```
pant mkdir <目录>
```

#### 例子
```
pant mkdir 123
```

## 删除文件/目录
```
pant rm <网盘文件或目录的路径1> <文件或目录2> <文件或目录3> ...
```

注意: 删除多个文件和目录时, 请确保每一个文件和目录都存在, 否则删除操作会失败.

被删除的文件或目录可在网盘文件回收站找回.

#### 例子
```
# 删除 /我的资源/1.mp4
pant rm /我的资源/1.mp4

# 删除 /我的资源/1.mp4 和 /我的资源/2.mp4
pant rm /我的资源/1.mp4 /我的资源/2.mp4

# 删除 /我的资源 内的所有文件和目录, 但不删除该目录
pant rm /我的资源/*

# 删除 /我的资源 整个目录 !!
pant rm /我的资源
```

## 拷贝文件/目录
```
pant cp <文件/目录> <目标 文件/目录>
pant cp <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>
```

注意: 拷贝多个文件和目录时, 请确保每一个文件和目录都存在, 否则拷贝操作会失败.

#### 例子
```
# 将 /我的资源/1.mp4 复制到 根目录 /
pant cp /我的资源/1.mp4 /

# 将 /我的资源/1.mp4 和 /我的资源/2.mp4 复制到 根目录 /
pant cp /我的资源/1.mp4 /我的资源/2.mp4 /
```

## 移动/重命名文件/目录
```
# 移动:
pant mv <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>
# 重命名:
pant mv <文件/目录> <重命名的文件/目录>
```

注意: 移动多个文件和目录时, 请确保每一个文件和目录都存在, 否则移动操作会失败.

#### 例子
```
# 将 /我的资源/1.mp4 移动到 根目录 /
pant mv /我的资源/1.mp4 /

# 将 /我的资源/1.mp4 重命名为 /我的资源/3.mp4
pant mv /我的资源/1.mp4 /我的资源/3.mp4
```

## 重命名单个文件/目录
```
pant rename <原文件/目录> <新文件名或目标路径>
pant rn <原文件/目录> <新文件名或目标路径>
```

直接填写新文件名即可在同一目录下完成原地重命名，也可填写完整网盘路径：
```
pant rename /我的资源/1.mp4 3.mp4
```

## 转存文件/目录
```
# 转存分享链接或整段分享口令文本里的文件到当前目录（公开免密链接可省略提取码）:
pant transfer [选项] <分享链接或整段分享文本> [提取码]
pant tf [选项] <分享链接或整段分享文本> [提取码]
```

### 可选参数
```
  --download     转存成功后自动下载到本地保存目录
  --collect      当分享包含多个文件/目录时，自动整合归档到新建子目录中转存
  --fs_id value  仅转存分享中的指定 fs_id 文件/目录（可用于绕过大文件夹单次数量限制）
```

注意: 转存文件默认保存到当前网盘工作目录下（可通过 `pant cd` 预先切换目标目录）.

#### 例子
```
# 转存公开免密分享链接（无需提取码）
pant transfer https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW

# 将带提取码的分享链接转存到当前目录
pant transfer https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW edv4
pant transfer https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW?pwd=edv4

# 直接传入整段百度网盘分享口令文本自动解析链接与提取码
pant transfer "通过百度网盘分享的文件：示例资料.zip 链接：https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW 提取码：edv4"

# 多文件整合归档转存，并在转存完成后自动下载到本地
pant transfer --collect --download https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW edv4

# 指定单个 fs_id 转存
pant transfer --fs_id 1013792297798440 https://pan.baidu.com/s/12L_ZZVNxz5f_2CccoyyVrW edv4
```

## 分享文件/目录
```
pant share
```

### 设置分享文件/目录
```
pant share set <文件/目录1> <文件/目录2> ...
pant share s <文件/目录1> <文件/目录2> ...
```

### 列出已分享文件/目录
```
pant share list
pant share l
```

### 取消分享文件/目录
```
pant share cancel <shareid_1> <shareid_2> ...
pant share c <shareid_1> <shareid_2> ...
```

目前只支持通过分享id (shareid) 来取消分享.

## 离线下载
```
pant offlinedl
pant clouddl
pant od
```

离线下载支持http/https/ftp/电驴/磁力链协议

离线下载同时进行的任务数量有限, 超出限制的部分将无法添加.

### 添加离线下载任务
```
pant offlinedl add -path=<离线下载文件保存的路径> 资源地址1 地址2 ...
```

添加任务成功之后, 返回离线下载的任务ID.

### 精确查询离线下载任务
```
pant offlinedl query 任务ID1 任务ID2 ...
```

### 查询离线下载任务列表
```
pant offlinedl list
```

### 取消离线下载任务
```
pant offlinedl cancel 任务ID1 任务ID2 ...
```

### 删除离线下载任务
```
pant offlinedl delete 任务ID1 任务ID2 ...

# 清空离线下载任务记录, 程序不会进行二次确认, 谨慎操作!!!
pant offlinedl delete -all
```

#### 例子
```
# 将百度和腾讯主页, 离线下载到根目录 /
pant offlinedl add -path=/ http://baidu.com http://qq.com

# 添加磁力链接任务
pant offlinedl add magnet:?xt=urn:btih:xxx

# 查询任务ID为 12345 的离线下载任务状态
pant offlinedl query 12345

# 取消任务ID为 12345 的离线下载任务
pant offlinedl cancel 12345
```

## 回收站
```
pant recycle
```

回收站操作.

### 列出回收站文件列表
```
pant recycle list
```

#### 可选参数
```
  --page value  回收站文件列表页数 (default: 1)
```

### 还原回收站文件或目录
```
pant recycle restore <fs_id 1> <fs_id 2> <fs_id 3> ...
```

根据文件/目录的 fs_id, 还原回收站指定的文件或目录.

### 删除回收站文件或目录/清空回收站
```
pant recycle delete [-all] <fs_id 1> <fs_id 2> <fs_id 3> ...
```

根据文件/目录的 fs_id 或 -all 参数, 删除回收站指定的文件或目录或清空回收站.

#### 例子
```
# 从回收站还原两个文件, 其中的两个文件的 fs_id 分别为 1013792297798440 和 643596340463870
pant recycle restore 1013792297798440 643596340463870

# 从回收站删除两个文件, 其中的两个文件的 fs_id 分别为 1013792297798440 和 643596340463870
pant recycle delete 1013792297798440 643596340463870

# 清空回收站, 程序不会进行二次确认, 谨慎操作!!!
pant recycle delete -all
```

## 显示程序环境变量
```
pant env
```

- `PANT_CONFIG_DIR`: 配置文件路径
- `PANT_VERBOSE`: 是否启用调试
- `PANT_ASCII`: 是否强制使用纯 7-bit ASCII 符号
- `PANT_PROMPT_STYLE`: 交互式 REPL 提示符风格（`starship` 默认极简双行 / `boxed` 圆角框线双行）
- `NO_COLOR` / `CLICOLOR` / `CLICOLOR_FORCE`: 终端彩色输出控制

## 显示和修改程序配置项
```
# 显示配置
pant config

# 设置配置
pant config set
```

注意: 配置文件所在的目录默认保存在用户配置目录（或若程序所在目录已存在 `pcs_config.json` 则优先使用程序所在目录）：

Windows: `%APPDATA%\pan-tools`

其他操作系统: `$HOME/.config/pan-tools`

可通过设置环境变量 `PANT_CONFIG_DIR` 指定配置文件存放的目录.

谨慎修改 `appid`, `user_agent`, `pcs_ua`, `pan_ua` 的值, 否则访问网盘服务器时, 可能会出现错误.

如上传遇到异常可尝试修改 `pcs_addr`, 目前已知的地址有:

```
pcs.baidu.com
c.pcs.baidu.com
c2.pcs.baidu.com
c3.pcs.baidu.com
c4.pcs.baidu.com
c5.pcs.baidu.com
d.pcs.baidu.com
```
v3.9.8后上传时支持动态获取pcs服务器, 理论上不需要手动配置. 如希望使用静态pcs服务器, 可配置打开`fix_pcs_addr`

`cache_size` 的值支持可选设置单位了, 单位不区分大小写, `b` 和 `B` 均表示字节的意思, 如 `64KB`, `1MB`, `32kb`, `65536b`, `65536`.

`max_download_rate`, `max_upload_rate` 的值支持可选设置单位了, 单位为每秒的传输速率, 后缀`/s` 可省略, 如 `2MB/s`, `2MB`, `2m`, `2mb` 均为一个意思.

普通用户请将`max_parallel`和`max_download_load`都设置为1, 调大线程数只会在短时间内提升下载速度, 且极易很快触发限速, 导致几小时至几天内账号在各客户端都接近0速. 本软件不支持普通用户提速.

SVIP用户建议`max_parallel`设置为10以上, 根据实际带宽可调大, 但不建议超过20, `max_download_load`设置为1 - 2, 实验表明可以稳定满速下载.

#### 例子
```
# 显示所有可以设置的值
pant config -h
pant config set -h

# 设置下载文件的储存目录
pant config set -savedir D:/Downloads

# 设置下载最大并发量为 15
pant config set -max_parallel 15

# 组合设置
pant config set -max_parallel 150 -savedir D:/Downloads

# 切换交互式 REPL 提示符风格 (starship 默认极简右对齐 / boxed 圆角框线)
pant config set -prompt_style boxed
```

## 测试通配符
```
pant match <通配符表达式>
```

测试通配符匹配路径, 操作成功则输出所有匹配到的路径.

#### 例子
```
# 匹配 /我的资源 目录下所有mp4格式的文件
pant match /我的资源/*.mp4
```

## 工具箱
```
pant tool
pant tool enc -method=<method> -key=<key> <文件1> [文件2...]
pant tool dec -method=<method> -key=<key> <文件1> [文件2...]
```

目前工具箱支持文件加解密（`enc` / `dec`，未指定 `-key` 时默认密钥为 `"pan-tools"`）、校验和计算（`sumfile`）、通配符匹配测试（`match`）、公网 IP 查询（`getip`）与北京时间显示（`showtime`）.

# 初级使用教程

新手建议: 直接运行 `pant`，进入交互式 REPL 模式（或运行 `pant ui` 进入全屏图形化终端文件管理器）；

REPL 交互模式下支持两套可选双行提示符：默认 `starship` 极简右对齐风格（首行显示当前用户名、会员等级、`~/路径` 与右侧网盘容量占比，第二行为 `❯ ` 输入行），以及 `boxed` 圆角框线风格（`╭─ pan-tools ▏ ● 用户名 [VIP: ...] [dir: ...] [quota: ...]` + `╰─❯ `），可通过 `config set -prompt_style boxed`（或环境变量 `PANT_PROMPT_STYLE=boxed` / 启动参数 `--prompt-style boxed`）自由切换。

以下例子的命令, 均为 cli交互模式下的命令

运行命令的正确操作: **输入命令, 按一下回车键 (键盘上的 Enter 键)**, 程序会接收到命令并输出结果

## 1. 查看程序使用说明

cli交互模式下, 运行命令 `help`

## 2. 登录百度帐号 (必做)

cli交互模式下, 运行命令 `login -h` (注意空格) 查看帮助

cli交互模式下, 运行命令 `login` 打开登录方式选择菜单（默认使用百度网盘 App / 百度 App 扫码登录，也可直接运行 `login -qr` 扫码登录，或选择粘贴 Cookies / 输入 BDUSS + STOKEN 登录）

## 3. 切换网盘工作目录

cli交互模式下, 运行命令 `cd /我的资源` 将工作目录切换为 `/我的资源` (前提: 该目录存在于网盘)

目录支持通配符匹配, 所以你也可以这样: 运行命令 `cd /我的*` 或 `cd /我的??` 将工作目录切换为 `/我的资源`, 简化输入.

将工作目录切换为 `/我的资源` 成功后, 运行命令 `cd ..` 切换上级目录, 即将工作目录切换为 `/`

为什么要这样设计呢, 举个例子,

假设 你要下载 `/我的资源` 内名为 `1.mp4` 和 `2.mp4` 两个文件, 而未切换工作目录, 你需要依次运行以下命令:

```
d /我的资源/1.mp4
d /我的资源/2.mp4
```

而切换网盘工作目录之后, 依次运行以下命令:

```
cd /我的资源
d 1.mp4
d 2.mp4
```

这样就达到了简化输入的目的

## 4. 网盘内列出文件和目录

cli交互模式下, 运行命令 `ls -h` (注意空格) 查看帮助

cli交互模式下, 运行命令 `ls` 来列出当前所在目录的文件和目录

cli交互模式下, 运行命令 `ls /我的资源` 来列出 `/我的资源` 内的文件和目录

cli交互模式下, 运行命令 `ls ..` 来列出当前所在目录的上级目录的文件和目录

## 5. 下载文件

说明: 下载的文件默认保存到用户主目录下的 `Downloads` 目录（Windows 下为程序所在目录的 `Downloads` 目录）

cli交互模式下, 运行命令 `d -h` (注意空格) 查看帮助

cli交互模式下, 运行命令 `d /我的资源/1.mp4` 来下载位于 `/我的资源/1.mp4` 的文件 `1.mp4` , 该操作等效于运行以下命令:

```
cd /我的资源
d 1.mp4
```

现在已经支持目录 (文件夹) 下载, 所以, 运行以下命令, 会下载 `/我的资源` 内的所有文件 (违规文件除外):

```
d /我的资源
```

## 6. 设置下载最大并发量

cli交互模式下, 运行命令 `config set -h` (注意空格) 查看设置帮助以及可供设置的值

cli交互模式下, 运行命令 `config set -max_parallel 2` 将下载最大并发量设置为 2

注意：普通用户下载最大并发量的值超过1将导致账号被限速; SVIP同样不宜设置过高, 建议10~20

## 7. 恢复默认配置

cli交互模式下, 运行命令 `config reset`

## 8. 退出程序

运行命令 `quit` 或 `exit` 或 组合键 `Ctrl+C` 或 组合键 `Ctrl+D`

# 已知问题

* 分片上传文件时, 当文件分片数大于1, 网盘端最终计算所得的md5值和本地的不一致, 这可能是百度网盘的bug, 测试把上传的文件下载到本地后，对比md5值是匹配的.
* 开启MD5校验下载时可能有 check MD5 不通过, 但文件其实并未出错的情况, 使用 `--nocheck` 下载或配置中启用 `no_check` 即可（默认已启用）.


# TODO

* 转存文件数量绕过单次限制

# 开源协议与致谢

本项目基于 Apache License 2.0 开源（详见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)），派生并深度重构自：
- `iikira/BaiduPCS-Go`（Copyright 2016-2020 iikira）
- [qjfoidnh/BaiduPCS-Go](https://github.com/qjfoidnh/BaiduPCS-Go)（Copyright 2018-2026 qjfoidnh）

在保留高速分片传输与网盘协议核心的基础上，`pan-tools`（`pant`）完成了模块化重构、现代终端 UI、Bubble Tea 全屏文件管理器、零依赖终端扫码登录，并移除了所有 GPL-3.0 外部依赖（以纯 Go 标准库与 HTTPS 网盘接口实现替代），使整个仓库保持纯粹的 Apache-2.0 协议。

# 交流反馈

提交 Issue: [qwerprog/pan-tools Issues](https://github.com/qwerprog/pan-tools/issues)
（上游历史 Issue 参考: [qjfoidnh/BaiduPCS-Go Issues](https://github.com/qjfoidnh/BaiduPCS-Go/issues)）
