package cliapp

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsupdate"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/getip"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/urfave/cli"
)

const cryptoDescription = `
	可用的方法 <method>:
		aes-128-ctr, aes-192-ctr, aes-256-ctr,
		aes-128-cfb, aes-192-cfb, aes-256-cfb,
		aes-128-ofb, aes-192-ofb, aes-256-ofb.

	密钥 <key>:
		aes-128 对应key长度为16, aes-192 对应key长度为24, aes-256 对应key长度为32,
		如果key长度不符合, 则自动修剪key, 舍弃超出长度的部分, 长度不足的部分用'\0'填充.

	GZIP <disable-gzip>:
		在文件加密之前, 启用GZIP压缩文件; 文件解密之后启用GZIP解压缩文件, 默认启用,
		如果不启用, 则无法检测文件是否解密成功, 解密文件时会保留源文件, 避免解密失败造成文件数据丢失.`

func runSumFileAction(c *cli.Context) error {
	if c.NArg() <= 0 {
		cli.ShowCommandHelp(c, c.Command.Name)
		return nil
	}

	var (
		firstErr error
		errItems []string
	)
	for k, filePath := range c.Args() {
		lp, err := checksum.GetFileSum(filePath, checksum.CHECKSUM_MD5|checksum.CHECKSUM_SLICE_MD5|checksum.CHECKSUM_CRC32)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			errItems = append(errItems, fmt.Sprintf("[%d] %s", k+1, err))
			continue
		}

		if !ui.IsQuiet() {
			ui.PrintInfo(os.Stdout, "[%d] %s", k+1, filePath)
		}

		strLength := strconv.FormatInt(lp.Length, 10)
		strMd5 := hex.EncodeToString(lp.MD5)
		strSliceMd5 := hex.EncodeToString(lp.SliceMD5)
		strCrc32 := strconv.FormatUint(uint64(lp.CRC32), 10)

		tb := pcstable.NewTable(os.Stdout)
		tb.SetColumnAlignment([]int{pcstable.AlignLeft, pcstable.AlignLeft})
		tb.AppendBulk([][]string{
			{"文件大小", strLength},
			{"md5", strMd5},
			{"前256KB切片的md5", strSliceMd5},
			{"crc32", strCrc32},
		})
		tb.Render()
		if !ui.IsQuiet() {
			fmt.Println()
		}
	}

	if len(errItems) == 1 && c.NArg() == 1 {
		return firstErr
	}
	if len(errItems) > 0 {
		return fmt.Errorf("计算文件校验失败 (%d/%d): %s (%w)", len(errItems), c.NArg(), strings.Join(errItems, "; "), firstErr)
	}
	return nil
}

func runMatchAction(c *cli.Context) error {
	if c.NArg() != 1 {
		cli.ShowCommandHelp(c, c.Command.Name)
		return nil
	}
	return pcscommand.RunTestShellPattern(c.Args()[0])
}

func systemCommands(app *cli.App) []cli.Command {
	sumfileCmd := cli.Command{
		Name:      "sumfile",
		Aliases:   []string{"sf"},
		Usage:     "获取本地文件的校验信息(大小、md5、前256KB切片md5、crc32)",
		UsageText: app.Name + " sumfile <本地文件的路径1> <本地文件的路径2> ...",
		Description: `
	获取本地文件的大小, md5, 前256KB切片的md5, crc32.

	示例:
	pant sumfile C:/Users/Administrator/Desktop/1.mp4
`,
		Category: "其他",
		Before:   reloadFn,
		Action:   runSumFileAction,
	}

	matchCmd := cli.Command{
		Name:      "match",
		Usage:     "测试通配符",
		UsageText: app.Name + " match <通配符表达式>",
		Description: `
	测试通配符匹配路径, 操作成功则输出所有匹配到的路径.

	示例:
	pant match /我的资源/*.mp4
`,
		Category: "百度网盘",
		Before:   reloadFn,
		Action:   runMatchAction,
	}

	toolSumfileCmd := sumfileCmd
	toolSumfileCmd.Category = ""
	toolMatchCmd := matchCmd
	toolMatchCmd.Category = ""

	return []cli.Command{
		{
			Name:     "run",
			Usage:    "执行系统命令",
			Category: "其他",
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				cmd := exec.Command(c.Args().First(), c.Args().Tail()...)
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr

				return cmd.Run()
			},
		},
		{
			Name:  "env",
			Usage: "显示程序环境变量",
			Description: `
	PANT_CONFIG_DIR: 配置文件路径,
	PANT_VERBOSE: 是否启用调试,
	PANT_ASCII: 是否强制使用纯 ASCII 符号,
	PANT_PROMPT_STYLE: 交互式 REPL 提示符风格 (starship / boxed),
	NO_COLOR / CLICOLOR / CLICOLOR_FORCE: 终端彩色输出控制.
`,
			Category: "其他",
			Action: func(c *cli.Context) error {
				envStr := "%s=\"%s\"\n"
				for _, key := range []string{pcsverbose.EnvVerbose, pcsconfig.EnvConfigDir, "PANT_ASCII", ui.EnvPromptStyle, "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE"} {
					if val, ok := os.LookupEnv(key); ok {
						fmt.Printf(envStr, key, val)
						continue
					}
					switch key {
					case pcsverbose.EnvVerbose:
						fmt.Printf(envStr, key, "0")
					case pcsconfig.EnvConfigDir:
						fmt.Printf(envStr, key, pcsconfig.GetConfigDir())
					default:
						fmt.Printf(envStr, key, "")
					}
				}
				return nil
			},
		},
		{
			Name:     "update",
			Usage:    "检测程序更新",
			Category: "其他",
			Action: func(c *cli.Context) error {
				if c.IsSet("y") {
					if !c.Bool("y") {
						return nil
					}
				}
				pcsupdate.CheckUpdate(app.Version, c.Bool("y"))
				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "y",
					Usage: "确认更新",
				},
			},
		},
		{
			Name:        "config",
			Usage:       "显示和修改程序配置项",
			Description: "显示和修改程序配置项",
			Category:    "配置",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				if !ui.IsQuiet() {
					ui.PrintInfo(os.Stdout, "当前配置（运行 '%s config set' 可修改配置项）", app.Name)
				}
				pcsconfig.Config.PrintTable()
				return nil
			},
			Subcommands: []cli.Command{
				{
					Name:      "set",
					Usage:     "修改程序配置项",
					UsageText: app.Name + " config set [arguments...]",
					Description: `
	注意:
		可通过设置环境变量 PANT_CONFIG_DIR, 指定配置文件存放的目录.

		谨慎修改 appid, user_agent, pcs_ua, pan_ua 的值, 否则访问网盘服务器时, 可能会出现错误
		cache_size 的值支持可选设置单位了, 单位不区分大小写, b 和 B 均表示字节的意思, 如 64KB, 1MB, 32kb, 65536b, 65536
		max_download_rate, max_upload_rate 的值支持可选设置单位了, 单位为每秒的传输速率, 后缀'/s' 可省略, 如 2MB/s, 2MB, 2m, 2mb 均为一个意思

	例子:
		pant config set -appid=266719
		pant config set -enable_https=false
		pant config set -prompt_style boxed
		pant config set -user_agent="netdisk;2.2.51.6;netdisk;10.0.63;PC;android-android"
		pant config set -cache_size 64KB
		pant config set -cache_size 16384 -max_parallel 200 -savedir D:/download`,
					Action: func(c *cli.Context) error {
						if c.NumFlags() <= 0 || c.NArg() > 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						// 先在零值临时配置上完成所有可失败项的原子校验，避免中途失败污染内存或落盘配置。
						var validator pcsconfig.PCSConfig
						if c.IsSet("pcs_addr") && !validator.SetPCSAddr(c.String("pcs_addr")) {
							return fmt.Errorf("设置 pcs_addr 错误: pcs服务器地址不合法")
						}
						if c.IsSet("cache_size") {
							if err := validator.SetCacheSizeByStr(c.String("cache_size")); err != nil {
								return fmt.Errorf("设置 cache_size 错误: %w", err)
							}
						}
						if c.IsSet("max_download_rate") {
							if err := validator.SetMaxDownloadRateByStr(c.String("max_download_rate")); err != nil {
								return fmt.Errorf("设置 max_download_rate 错误: %w", err)
							}
						}
						if c.IsSet("max_upload_rate") {
							if err := validator.SetMaxUploadRateByStr(c.String("max_upload_rate")); err != nil {
								return fmt.Errorf("设置 max_upload_rate 错误: %w", err)
							}
						}
						if c.IsSet("prompt_style") {
							if err := validator.SetPromptStyle(c.String("prompt_style")); err != nil {
								return fmt.Errorf("设置 prompt_style 错误: %w", err)
							}
						}

						if c.IsSet("appid") {
							pcsconfig.Config.SetAppID(c.Int("appid"))
						}
						if c.IsSet("enable_https") {
							pcsconfig.Config.SetEnableHTTPS(c.Bool("enable_https"))
						}
						if c.IsSet("no_check") {
							pcsconfig.Config.SetNoCheck(c.Bool("no_check"))
						}
						if c.IsSet("ignore_illegal") {
							pcsconfig.Config.SetIgnoreIllegal(c.Bool("ignore_illegal"))
						}
						if c.IsSet("force_login_username") {
							pcsconfig.Config.SetForceLogin(c.String("force_login_username"))
						}
						if c.IsSet("user_agent") {
							pcsconfig.Config.SetUserAgent(c.String("user_agent"))
						}
						if c.IsSet("pcs_ua") {
							pcsconfig.Config.SetPCSUA(c.String("pcs_ua"))
						}
						if c.IsSet("pcs_addr") {
							pcsconfig.Config.SetPCSAddr(c.String("pcs_addr"))
						}
						if c.IsSet("fix_pcs_addr") {
							pcsconfig.Config.SetStaticPCSAddr(c.Bool("fix_pcs_addr"))
						}
						if c.IsSet("upload_policy") {
							pcsconfig.Config.SetUploadPolicy(c.String("upload_policy"))
						}
						if c.IsSet("pan_ua") {
							pcsconfig.Config.SetPanUA(c.String("pan_ua"))
						}
						if c.IsSet("cache_size") {
							_ = pcsconfig.Config.SetCacheSizeByStr(c.String("cache_size"))
						}
						if c.IsSet("max_parallel") {
							pcsconfig.Config.MaxParallel = c.Int("max_parallel")
						}
						if c.IsSet("max_upload_parallel") {
							pcsconfig.Config.MaxUploadParallel = c.Int("max_upload_parallel")
						}
						if c.IsSet("max_download_load") {
							pcsconfig.Config.MaxDownloadLoad = c.Int("max_download_load")
						}
						if c.IsSet("max_upload_load") {
							pcsconfig.Config.MaxUploadLoad = c.Int("max_upload_load")
						}
						if c.IsSet("max_download_rate") {
							_ = pcsconfig.Config.SetMaxDownloadRateByStr(c.String("max_download_rate"))
						}
						if c.IsSet("max_upload_rate") {
							_ = pcsconfig.Config.SetMaxUploadRateByStr(c.String("max_upload_rate"))
						}
						if c.IsSet("savedir") {
							pcsconfig.Config.SaveDir = c.String("savedir")
						}
						if c.IsSet("proxy") {
							pcsconfig.Config.SetProxy(c.String("proxy"))
						}
						if c.IsSet("proxy_hostnames") {
							pcsconfig.Config.SetProxyHostnames(c.String("proxy_hostnames"))
						}
						if c.IsSet("local_addrs") {
							pcsconfig.Config.SetLocalAddrs(c.String("local_addrs"))
						}
						if c.IsSet("prompt_style") {
							_ = pcsconfig.Config.SetPromptStyle(c.String("prompt_style"))
						}

						err := pcsconfig.Config.Save()
						if err != nil {
							return err
						}
						if c.IsSet("prompt_style") {
							promptStyleReconfigured.Store(true)
						}

						if !ui.IsQuiet() {
							pcsconfig.Config.PrintTable()
							ui.PrintSuccess(os.Stdout, "保存配置成功")
						}

						return nil
					},
					Flags: []cli.Flag{
						cli.IntFlag{
							Name:  "appid",
							Usage: "百度 PCS 应用ID",
						},
						cli.StringFlag{
							Name:  "cache_size",
							Usage: "下载缓存",
						},
						cli.IntFlag{
							Name:  "max_parallel",
							Usage: "下载网络全部连接的最大并发量",
						},
						cli.IntFlag{
							Name:  "max_upload_parallel",
							Usage: "上传网络单个连接的最大并发量",
						},
						cli.IntFlag{
							Name:  "max_download_load",
							Usage: "同时进行下载文件的最大数量",
						},
						cli.IntFlag{
							Name:  "max_upload_load",
							Usage: "同时进行上传文件的最大数量",
						},
						cli.StringFlag{
							Name:  "max_download_rate",
							Usage: "限制最大下载速度, 0代表不限制",
						},
						cli.StringFlag{
							Name:  "max_upload_rate",
							Usage: "限制最大上传速度, 0代表不限制",
						},
						cli.StringFlag{
							Name:  "savedir",
							Usage: "下载文件的储存目录",
						},
						cli.BoolFlag{
							Name:  "enable_https",
							Usage: "启用 https",
						},
						cli.BoolFlag{
							Name:  "ignore_illegal",
							Usage: "忽略上传时文件名中的非法字符",
						},
						cli.StringFlag{
							Name:  "force_login_username",
							Usage: "强制登录指定用户名, 只适用于网盘用户信息接口失效的情况",
						},
						cli.BoolFlag{
							Name:  "no_check",
							Usage: "关闭下载文件md5校验",
						},
						cli.StringFlag{
							Name:  "upload_policy",
							Usage: "设置上传遇到同名文件时的策略",
						},
						cli.StringFlag{
							Name:  "user_agent",
							Usage: "浏览器标识",
						},
						cli.StringFlag{
							Name:  "pcs_ua",
							Usage: "PCS 浏览器标识",
						},
						cli.StringFlag{
							Name:  "pcs_addr",
							Usage: "PCS 服务器地址",
						},
						cli.BoolFlag{
							Name:  "fix_pcs_addr",
							Usage: "使用静态PCS 服务器",
						},
						cli.StringFlag{
							Name:  "pan_ua",
							Usage: "Pan 浏览器标识",
						},
						cli.StringFlag{
							Name:  "proxy",
							Usage: "设置代理, 支持 http/socks5 代理",
						},
						cli.StringFlag{
							Name:  "proxy_hostnames",
							Usage: "设置走代理的域名范围, 多个域名用逗号隔开, 留空则所有域名均代理",
						},
						cli.StringFlag{
							Name:  "local_addrs",
							Usage: "设置本地网卡地址, 多个地址用逗号隔开",
						},
						cli.StringFlag{
							Name:  "prompt_style, prompt-style",
							Usage: "设置交互式提示符风格 (starship / boxed)",
						},
					},
				},
				{
					Name:      "reset",
					Usage:     "恢复默认配置项",
					UsageText: app.Name + " config reset",
					Action: func(c *cli.Context) error {
						pcsconfig.Config.InitDefaultConfig()
						err := pcsconfig.Config.Save()
						if err != nil {
							return err
						}
						_ = pcsconfig.Config.Reload()
						promptStyleReconfigured.Store(true)
						if !ui.IsQuiet() {
							pcsconfig.Config.PrintTable()
							ui.PrintSuccess(os.Stdout, "恢复默认配置成功")
						}
						return nil
					},
				},
			},
		},
		sumfileCmd,
		matchCmd,
		{
			Name:     "tool",
			Usage:    "工具箱",
			Category: "其他",
			Action: func(c *cli.Context) error {
				cli.ShowCommandHelp(c, c.Command.Name)
				return nil
			},
			Subcommands: []cli.Command{
				toolSumfileCmd,
				toolMatchCmd,
				{
					Name:  "showtime",
					Usage: "显示当前时间(北京时间)",
					Action: func(c *cli.Context) error {
						fmt.Print(pcsutil.BeijingTimeOption("printLog"))
						return nil
					},
				},
				{
					Name:  "getip",
					Usage: "获取IP地址",
					Action: func(c *cli.Context) error {
						fmt.Printf("内网IP地址: \n")
						for _, address := range pcsutil.ListAddresses() {
							fmt.Printf("%s\n", address)
						}
						fmt.Printf("\n")

						ipAddr, err := getip.IPInfoFromTechainBaiduByClient(pcsconfig.Config.HTTPClient())
						if err != nil {
							return fmt.Errorf("获取公网IP错误: %w", err)
						}

						fmt.Printf("公网IP地址: %s\n", ipAddr)
						return nil
					},
				},
				{
					Name:        "enc",
					Usage:       "加密文件",
					UsageText:   app.Name + " tool enc -method=<method> -key=<key> [files...]",
					Description: cryptoDescription,
					Action: func(c *cli.Context) error {
						if c.NArg() <= 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						var (
							firstErr error
							errItems []string
						)
						for _, filePath := range c.Args() {
							encryptedFilePath, err := pcsutil.EncryptFile(c.String("method"), []byte(c.String("key")), filePath, !c.Bool("disable-gzip"))
							if err != nil {
								if firstErr == nil {
									firstErr = err
								}
								errItems = append(errItems, fmt.Sprintf("%s: %s", filePath, err))
								continue
							}

							if !ui.IsQuiet() {
								ui.PrintSuccess(os.Stdout, "加密成功: %s -> %s", filePath, encryptedFilePath)
							}
						}

						if len(errItems) == 1 && c.NArg() == 1 {
							return firstErr
						}
						if len(errItems) > 0 {
							return fmt.Errorf("加密文件失败 (%d/%d): %s (%w)", len(errItems), c.NArg(), strings.Join(errItems, "; "), firstErr)
						}
						return nil
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "method",
							Usage: "加密方法",
							Value: "aes-128-ctr",
						},
						cli.StringFlag{
							Name:  "key",
							Usage: "加密密钥",
							Value: "pan-tools",
						},
						cli.BoolFlag{
							Name:  "disable-gzip",
							Usage: "不启用GZIP",
						},
					},
				},
				{
					Name:        "dec",
					Usage:       "解密文件",
					UsageText:   app.Name + " tool dec -method=<method> -key=<key> [files...]",
					Description: cryptoDescription,
					Action: func(c *cli.Context) error {
						if c.NArg() <= 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						var (
							firstErr error
							errItems []string
						)
						for _, filePath := range c.Args() {
							decryptedFilePath, err := pcsutil.DecryptFile(c.String("method"), []byte(c.String("key")), filePath, !c.Bool("disable-gzip"))
							if err != nil {
								if firstErr == nil {
									firstErr = err
								}
								errItems = append(errItems, fmt.Sprintf("%s: %s", filePath, err))
								continue
							}

							if !ui.IsQuiet() {
								ui.PrintSuccess(os.Stdout, "解密成功: %s -> %s", filePath, decryptedFilePath)
							}
						}

						if len(errItems) == 1 && c.NArg() == 1 {
							return firstErr
						}
						if len(errItems) > 0 {
							return fmt.Errorf("解密文件失败 (%d/%d): %s (%w)", len(errItems), c.NArg(), strings.Join(errItems, "; "), firstErr)
						}
						return nil
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "method",
							Usage: "加密方法",
							Value: "aes-128-ctr",
						},
						cli.StringFlag{
							Name:  "key",
							Usage: "加密密钥",
							Value: "pan-tools",
						},
						cli.BoolFlag{
							Name:  "disable-gzip",
							Usage: "不启用GZIP",
						},
					},
				},
			},
		},
		{
			Name:        "clear",
			Aliases:     []string{"cls"},
			Usage:       "清空控制台",
			UsageText:   app.Name + " clear",
			Description: "清空控制台屏幕",
			Category:    "其他",
			Action: func(c *cli.Context) error {
				pcsliner.ClearScreen()
				return nil
			},
		},
		{
			Name:      "?",
			Aliases:   []string{"？"},
			Usage:     "显示帮助信息 (同 help)",
			ArgsUsage: "[command]",
			Hidden:    true,
			HideHelp:  true,
			Action: func(c *cli.Context) error {
				args := c.Args()
				if args.Present() {
					return cli.ShowCommandHelp(c, args.First())
				}
				cli.ShowAppHelp(c)
				return nil
			},
		},
		{
			Name:    "quit",
			Aliases: []string{"exit"},
			Usage:   "退出程序",
			Action: func(c *cli.Context) error {
				return cli.NewExitError("", 0)
			},
			Hidden:   true,
			HideHelp: true,
		},
	}
}
