// Package cliapp 提供 pan-tools (pant) 命令行应用构建与交互式 REPL 实现
package cliapp

import (
	"errors"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/urfave/cli"
)

func init() {
	ui.InstallHelpTemplates()
}

var (
	reloadFn = func(c *cli.Context) error {
		err := pcsconfig.Config.Reload()
		if err != nil {
			ui.PrintWarn(os.Stderr, "重载配置错误: %s", err)
		}
		return nil
	}

	saveFunc = func(c *cli.Context) error {
		err := pcsconfig.Config.Save()
		if err != nil {
			ui.PrintWarn(os.Stderr, "保存配置错误: %s", err)
		}
		return nil
	}
)

// InitConfig 初始化工作目录和全局配置
func InitConfig() {
	pcsutil.ChWorkDir()

	err := pcsconfig.Config.Init()
	switch err {
	case nil:
	case pcsconfig.ErrConfigFileNoPermission, pcsconfig.ErrConfigContentsParseError:
		ui.PrintError(os.Stderr, "配置文件读取失败", err, "检查配置文件权限或修复 JSON 语法错误")
		os.Exit(1)
	default:
		ui.PrintWarn(os.Stderr, "config init warning: %s", err)
	}
}

// NewApp 创建并配置 pan-tools CLI App 实例
func NewApp(version string) *cli.App {
	app := cli.NewApp()
	app.Name = ui.AppName
	app.HelpName = ui.AppName
	app.Version = version
	app.Author = "pan-tools Contributors (based on iikira & qjfoidnh/BaiduPCS-Go)"
	app.Copyright = "Apache-2.0 License · (c) 2016-2026 iikira, qjfoidnh & pan-tools Contributors"
	app.Usage = "现代百度网盘 Go 终端客户端 (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	app.Description = `pan-tools (pant) 是基于 Go 编写的现代百度网盘命令行与交互式终端客户端.
	承袭高速分块下载与秒传核心引擎, 提供现代化终端美学、结构化输出与自动补全体验.

	特色功能:
		网盘路径漫游与通配符匹配 (ls / cd / search / tree);
		高并发分片下载、秒传检验、断点续传、离线下载与分享转存;
		支持 --json / --plain 管道自动化输出与 NO_COLOR / 纯 ASCII 降级.`

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "verbose",
			Usage:  "启用调试日志",
			EnvVar: pcsverbose.EnvVerbose,
		},
		cli.BoolFlag{
			Name:  "no-color",
			Usage: "禁用终端 ANSI 彩色输出 (兼容 NO_COLOR 环境变量)",
		},
		cli.BoolFlag{
			Name:  "ascii",
			Usage: "强制使用纯 7-bit ASCII 符号边框与字符徽标 (兼容 PANT_ASCII=1)",
		},
		cli.StringFlag{
			Name:  "prompt-style, prompt_style",
			Usage: "指定交互式 REPL 提示符风格 (可选值: starship, boxed; 兼容 PANT_PROMPT_STYLE)",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "静默模式，隐藏 REPL 启动欢迎横幅与实时进度条",
		},
	}

	app.Before = func(c *cli.Context) error {
		// 仅向上开启全局状态，确保进入 REPL 前指定的全局参数在子命令调用期间不丢失。
		if c.GlobalBool("verbose") {
			pcsverbose.IsVerbose = true
		}
		if c.GlobalBool("no-color") {
			ui.SetNoColor(true)
		}
		if c.GlobalBool("ascii") {
			ui.SetASCIIOnly(true)
		}
		if c.GlobalBool("quiet") {
			ui.SetQuiet(true)
		}
		if c.GlobalIsSet("prompt-style") {
			parsed, err := ui.ParsePromptStyle(c.GlobalString("prompt-style"))
			if err != nil {
				return err
			}
			ui.SetPromptStyleOverride(string(parsed))
		}
		return nil
	}

	app.ErrWriter = os.Stderr
	app.OnUsageError = func(c *cli.Context, err error, isSubcommand bool) error {
		return err
	}
	app.Action = runREPL

	var cmds []cli.Command
	cmds = append(cmds, accountCommands(app)...)
	cmds = append(cmds, fsCommands(app)...)
	cmds = append(cmds, transferCommands(app)...)
	cmds = append(cmds, cloudCommands(app)...)
	cmds = append(cmds, systemCommands(app)...)
	attachUsageErrorHandler(cmds)
	app.Commands = cmds

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	return app
}

func attachUsageErrorHandler(cmds []cli.Command) {
	for i := range cmds {
		if cmds[i].OnUsageError == nil {
			cmds[i].OnUsageError = func(c *cli.Context, err error, isSubcommand bool) error {
				return err
			}
		}
		if len(cmds[i].Subcommands) > 0 {
			attachUsageErrorHandler(cmds[i].Subcommands)
		}
	}
}

// preScanGlobalFlags 在 InitConfig() 与 urfave/cli help / OnUsageError 运行前预先识别顶层全局标志；
// 遇到 "--" 或首个非标志子命令词元（如 run / offlinedl）立即停止扫描，防止将子命令参数（如 grep -q）误识别为全局标志。
func preScanGlobalFlags(args []string) {
	for i := 1; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			break
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			break
		}

		trimmed := strings.TrimLeft(tok, "-")
		name := trimmed
		eqVal := ""
		hasEq := false
		if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
			name = trimmed[:eq]
			eqVal = trimmed[eq+1:]
			hasEq = true
		}

		if name == "prompt-style" || name == "prompt_style" {
			rawVal := eqVal
			if !hasEq {
				if i+1 >= len(args) {
					break
				}
				i++
				rawVal = args[i]
			}
			if parsed, err := ui.ParsePromptStyle(rawVal); err == nil {
				ui.SetPromptStyleOverride(string(parsed))
			}
			continue
		}

		val := true
		if hasEq {
			parsed, err := strconv.ParseBool(eqVal)
			if err != nil {
				continue
			}
			val = parsed
		}

		switch name {
		case "no-color":
			ui.SetNoColor(val)
		case "ascii":
			ui.SetASCIIOnly(val)
		case "quiet", "q":
			ui.SetQuiet(val)
		case "verbose":
			pcsverbose.IsVerbose = val
		}
	}
}

func outputFlags() []cli.Flag {
	return []cli.Flag{
		cli.BoolFlag{
			Name:  "json",
			Usage: "以结构化 JSON 格式输出结果 (适配管道与脚本)",
		},
		cli.BoolFlag{
			Name:  "plain",
			Usage: "以纯制表符分隔(TSV)格式输出，无边框和颜色",
		},
	}
}

func outputMode(c *cli.Context) ui.OutputMode {
	return ui.ResolveOutputMode(c.Bool("json"), c.Bool("plain"))
}

// Run 初始化配置并执行 CLI 应用，所有未处理的命令错误统一输出到 stderr。
func Run(version string, args []string) error {
	preScanGlobalFlags(args)
	InitConfig()
	defer pcsconfig.Config.Close()

	app := NewApp(version)
	err := app.Run(args)
	if err != nil {
		var exitCoder cli.ExitCoder
		if !errors.As(err, &exitCoder) || exitCoder.ExitCode() != 0 {
			ui.PrintError(os.Stderr, "命令执行失败", err, "")
		}
	}
	return err
}
