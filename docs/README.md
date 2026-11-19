# pan-tools (`pant`) 架构与接口文档说明

## 1. 当前分层架构概览

pan-tools 采用自底向上的单向依赖分层，严禁底层包反向导入上层业务或终端 UI 包：

```text
main.go
  └── internal/cliapp           # CLI 命令注册、全局标志 (--quiet/--no-color/--ascii/--prompt-style/--verbose)、子命令结构化输出 (--json/--plain)、REPL 补全
        ├── internal/tui        # Bubble Tea 全屏交互式网盘文件管理器 (pant ui / tui / browse)
        ├── internal/pcscommand # 各网盘命令编排层 (ls/cd/download/upload/transfer/share/recycle/offlinedl...)
        │     ├── internal/pcsfunctions/{pcsdownload,pcsupload}            # 传输任务单元与断点数据库
        │     └── internal/pcsconfig                                       # 多账号会话与本地配置持久化
        ├── internal/pcsupdate  # GitHub Release SemVer 自更新与 Zip 安全解压
        ├── pcsutil/getip       # 公网 IP 探测（依赖 requester HTTPClient）
        └── pcstable -> internal/ui # Atom One Dark 主题、响应式横幅、可选双行提示符 (starship/boxed)、TransferDashboard 并发看板与自适应表格
              │
              ▼
        baidupcs (+ pcserror / netdisksign / expires) # 纯百度网盘 SDK 层（无终端渲染依赖）
              │
              ▼
        requester (+ downloader / uploader / rio / transfer / multipartreader) # HTTP 客户端、连接池、多线程分片传输与限速
              │
              ▼
        pcsutil (+ checksum / converter / taskframework) & pcsverbose # 基础通用工具层（纯标准库加密/压缩、校验与任务调度）
```

## 2. 实际使用的百度网盘服务端接口拓扑

当前版本通过用户登录态 Cookie（`BDUSS` + `STOKEN`）协同调用两类接口（定义见 `baidupcs/prepare*.go`）：

| 域名与前缀 | 用途范围 | 典型接口 |
| :--- | :--- | :--- |
| `pan.baidu.com/api/*` & `/share/*` | 网盘 Web API（支持超过 1000 条翻页、秒传握手、分享与回收站、会员等级查询） | `/api/list`、`/api/precreate`、`/api/create`、`/api/quota`、`/rest/2.0/membership/user`、`/share/pset`、`/share/transfer`、`/api/recycle/*` |
| `pcs.baidu.com/rest/2.0/pcs/*`（含动态上传节点） | PCS 数据流与批量文件操作接口 | `file?method=locatedownload`、`file?method=meta/search/mkdir/copy/move/delete/restore`、`superfile2?method=upload`、`services/cloud_dl` |

## 3. 历史归档文档（`docs/archive/`）

`docs/archive/` 目录保留了早期（约 2013 年）百度开放平台 PCS OAuth REST API 与结构化数据（Structured Data API）参考资料，仅供历史对照；其中结构化数据接口与早期 OAuth 流程并非当前 pan-tools 客户端的实际运行协议：

- [PCS 历史综述](./archive/overview.md)
- [PCS 文件 API 历史列表](./archive/file_data_apis_list.md)
- [PCS 文件 API 历史错误码](./archive/file_data_apis_error.md)
- [结构化数据 API 历史综述（未使用）](./archive/structured_data_apis_overview.md)
- [结构化数据 API 历史列表（未使用）](./archive/structured_data_api_list.md)
- [结构化数据 API 历史错误码（未使用）](./archive/structured_data_apis_error.md)
