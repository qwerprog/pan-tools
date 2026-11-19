package cliapp

import (
	"fmt"
	"path/filepath"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions/pcsdownload"
	"github.com/urfave/cli"
)

func transferCommands(app *cli.App) []cli.Command {
	return []cli.Command{
		{
			Name:      "download",
			Aliases:   []string{"d"},
			Usage:     "下载文件/目录",
			UsageText: app.Name + " download <文件/目录路径1> <文件/目录2> <文件/目录3> ...",
			Description: `
	下载的文件默认保存到用户主目录下的 Downloads 目录 (Windows 为程序所在目录的 Downloads 目录).
	通过 pant config set -savedir <savedir>, 自定义保存的目录.
	支持多个文件或目录下载.
	支持下载完成后自动校验文件, 但并不是所有的文件都支持校验!
	自动跳过下载重名的文件!

	下载模式说明:
		pcs: 通过百度网盘的 PCS API 下载, locate模式提示user is not authorized可尝试此模式
		stream: 通过百度网盘的 PCS API, 以流式文件的方式下载, 效果同 pcs
		locate: 默认的下载模式。从百度网盘 Android 客户端, 获取下载链接的方式来下载

	示例:

	设置保存目录, 保存到 D:\Downloads
	注意区别反斜杠 "\" 和 斜杠 "/" !!!
	pant config set -savedir D:\\Downloads
	或者
	pant config set -savedir D:/Downloads

	下载 /我的资源/1.mp4
	pant d /我的资源/1.mp4

	下载 /我的资源 整个目录!!
	pant d /我的资源

	下载网盘内的全部文件!!
	pant d /
	pant d *
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				// 处理saveTo
				var saveTo string
				if c.Bool("save") {
					saveTo = "."
				} else if c.String("saveto") != "" {
					saveTo = filepath.Clean(c.String("saveto"))
				}

				// 处理解析downloadMode
				var downloadMode pcsdownload.DownloadMode
				switch c.String("mode") {
				case "pcs":
					downloadMode = pcsdownload.DownloadModePCS
				case "stream":
					downloadMode = pcsdownload.DownloadModeStreaming
				case "locate":
					downloadMode = pcsdownload.DownloadModeLocate
				default:
					return fmt.Errorf("下载方式解析失败: %s", c.String("mode"))
				}

				do := &pcscommand.DownloadOptions{
					IsTest:               c.Bool("test"),
					IsPrintStatus:        c.Bool("status"),
					IsExecutedPermission: c.Bool("x"),
					IsOverwrite:          c.Bool("ow"),
					DownloadMode:         downloadMode,
					SaveTo:               saveTo,
					Parallel:             c.Int("p"),
					Load:                 c.Int("l"),
					MaxRetry:             c.Int("retry"),
					NoCheck:              c.Bool("nocheck"),
					LinkPrefer:           c.Int("dindex"),
					ModifyMTime:          c.Bool("mtime"),
					FullPath:             c.Bool("fullpath"),
				}

				return pcscommand.RunDownload(c.Args(), do)
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "test",
					Usage: "测试下载, 此操作不会保存文件到本地",
				},
				cli.BoolFlag{
					Name:  "ow",
					Usage: "overwrite, 覆盖已存在的文件",
				},
				cli.BoolFlag{
					Name:  "status",
					Usage: "输出所有线程的工作状态",
				},
				cli.BoolFlag{
					Name:  "save",
					Usage: "将下载的文件直接保存到当前工作目录",
				},
				cli.StringFlag{
					Name:  "saveto",
					Usage: "将下载的文件直接保存到指定的目录",
				},
				cli.BoolFlag{
					Name:  "x",
					Usage: "为文件加上执行权限, (windows系统无效)",
				},
				cli.StringFlag{
					Name:  "mode",
					Usage: "下载模式, 可选值: pcs, stream, locate, 默认为 locate, 相关说明见上面的帮助",
					Value: "locate",
				},
				cli.IntFlag{
					Name:  "p",
					Usage: "指定下载线程数",
				},
				cli.IntFlag{
					Name:  "l",
					Usage: "指定同时进行下载文件的数量",
				},
				cli.IntFlag{
					Name:  "retry",
					Usage: "下载失败最大重试次数",
					Value: pcsdownload.DefaultDownloadMaxRetry,
				},
				cli.BoolFlag{
					Name:  "nocheck",
					Usage: "下载文件完成后不校验文件",
				},
				cli.BoolFlag{
					Name:  "mtime",
					Usage: "将本地文件的修改时间设置为服务器上的修改时间",
				},
				cli.IntFlag{
					Name:  "dindex",
					Usage: "使用备选下载链接中的第几个，默认第一个",
				},
				cli.BoolFlag{
					Name:  "fullpath",
					Usage: "以网盘完整路径保存到本地",
				},
			},
		},
		{
			Name:      "upload",
			Aliases:   []string{"u"},
			Usage:     "上传文件/目录",
			UsageText: app.Name + " upload <本地文件/目录的路径1> <文件/目录2> <文件/目录3> ... <目标目录>",
			Description: `
	上传默认采用分片并发上传的方式, 上传的文件将会保存到 <目标目录>.
	当上传的文件名和网盘的目录名称相同时, 不会覆盖目录, 防止丢失数据.

	示例:

	1. 将本地的 C:\Users\Administrator\Desktop\1.mp4 上传到网盘 /视频 目录
	注意区别反斜杠 "\" 和 斜杠 "/" !!!
	pant upload C:/Users/Administrator/Desktop/1.mp4 /视频

	2. 将本地的 C:\Users\Administrator\Desktop\1.mp4 和 C:\Users\Administrator\Desktop\2.mp4 上传到网盘 /视频 目录
	pant upload C:/Users/Administrator/Desktop/1.mp4 C:/Users/Administrator/Desktop/2.mp4 /视频

	3. 将本地的 C:\Users\Administrator\Desktop 整个目录上传到网盘 /视频 目录
	pant upload C:/Users/Administrator/Desktop /视频

	4. 使用相对路径
	pant upload 1.mp4 /视频
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() < 2 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				subArgs := c.Args()
				return pcscommand.RunUpload(subArgs[:c.NArg()-1], subArgs[c.NArg()-1], &pcscommand.UploadOptions{
					Parallel:      c.Int("p"),
					MaxRetry:      c.Int("retry"),
					Load:          c.Int("l"),
					NoRapidUpload: c.Bool("norapid"),
					Policy:        c.String("policy"),
				})
			},
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "p",
					Usage: "指定单个文件上传的最大线程数",
				},
				cli.IntFlag{
					Name:  "retry",
					Usage: "上传失败最大重试次数",
					Value: pcscommand.DefaultUploadMaxRetry,
				},
				cli.IntFlag{
					Name:  "l",
					Usage: "指定同时上传的最大文件数",
				},
				cli.BoolFlag{
					Name:  "norapid",
					Usage: "跳过秒传",
				},
				cli.StringFlag{
					Name:  "policy",
					Usage: fmt.Sprintf("对同名文件的处理策略 (default: %s), %s, %s", baidupcs.SkipPolicy, baidupcs.OverWritePolicy, baidupcs.RsyncPolicy),
				},
			},
		},
		{
			Name:      "locate",
			Aliases:   []string{"lt"},
			Usage:     "获取下载直链",
			UsageText: app.Name + " locate <文件1> <文件2> ...",
			Description: fmt.Sprintf(`
	获取下载直链

	若该功能无法正常使用, 提示"user is not authorized, hitcode:xxx", 尝试更换 User-Agent 为 %s:
	pant config set -user_agent "%s"
`, baidupcs.NetdiskUA, baidupcs.NetdiskUA),
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				opt := &pcscommand.LocateDownloadOption{
					FromPan: c.Bool("pan"),
				}

				return pcscommand.RunLocateDownload(c.Args(), opt)
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "pan",
					Usage: "从百度网盘首页获取下载链接",
				},
			},
		},
	}
}
