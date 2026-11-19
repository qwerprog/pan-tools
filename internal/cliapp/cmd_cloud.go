package cliapp

import (
	"fmt"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/urfave/cli"
)

func cloudCommands(app *cli.App) []cli.Command {
	return []cli.Command{
		{
			Name:      "transfer",
			Aliases:   []string{"tf"},
			Usage:     "转存文件/目录",
			UsageText: app.Name + " transfer [options] <分享链接或整段分享文本> [提取码]",
			Category:  "百度网盘",
			Before:    reloadFn,
			Description: `
	转存百度网盘分享链接中的文件或目录到当前网盘工作目录下.
	支持公开免密分享链接、带提取码的完整链接以及整段百度网盘分享口令文本.

	示例:
	pant transfer https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ
	pant transfer https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ 704e
	pant transfer https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ?pwd=704e
	pant transfer "通过百度网盘分享的文件：示例资料.zip 链接：https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ 提取码：704e"
	pant transfer --collect --download https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ 704e
`,
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}
				opt := &baidupcs.TransferOption{
					Download: c.Bool("download"),
					Collect:  c.Bool("collect"),
					FsID:     c.String("fs_id"),
				}
				return pcscommand.RunShareTransfer(c.Args(), opt)
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "download",
					Usage: "转存后直接下载到本地默认目录",
				},
				cli.BoolFlag{
					Name:  "collect",
					Usage: "多文件整合到一个文件夹中转存",
				},
				cli.StringFlag{
					Name:  "fs_id",
					Usage: "指定特定fs_id进行转存(用于绕过大文件夹数量限制)",
				},
			},
		},
		{
			Name:      "share",
			Aliases:   []string{"sh"},
			Usage:     "分享文件/目录",
			UsageText: app.Name + " share",
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				cli.ShowCommandHelp(c, c.Command.Name)
				return nil
			},
			Subcommands: []cli.Command{
				{
					Name:        "set",
					Aliases:     []string{"s"},
					Usage:       "设置分享文件/目录",
					UsageText:   app.Name + " share set <文件/目录1> <文件/目录2> ...",
					Description: `支持任意有效天数, 支持自定义提取码.`,
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}
						opt := &baidupcs.ShareOption{
							Password:   c.String("p"),
							Period:     c.Int("period"),
							IsCombined: c.Bool("f"),
						}
						return pcscommand.RunShareSet(c.Args(), opt)
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "p",
							Usage: "提取码",
							Value: "",
						},
						cli.IntFlag{
							Name:  "period",
							Usage: "有效天数, 0为永久",
							Value: 0,
						},
						cli.BoolFlag{
							Name:  "f",
							Usage: "输出带密码的完整链接格式",
						},
					},
				},
				{
					Name:      "list",
					Aliases:   []string{"l"},
					Usage:     "列出已分享文件/目录",
					UsageText: app.Name + " share list",
					Action: func(c *cli.Context) error {
						return pcscommand.RunShareListWithOutput(c.Int("page"), outputMode(c))
					},
					Flags: append([]cli.Flag{
						cli.IntFlag{
							Name:  "page",
							Usage: "分享列表的页数",
							Value: 1,
						},
					}, outputFlags()...),
				},
				{
					Name:        "cancel",
					Aliases:     []string{"c"},
					Usage:       "取消分享文件/目录",
					UsageText:   app.Name + " share cancel <shareid_1> <shareid_2> ...",
					Description: `目前只支持通过分享id (shareid) 来取消分享.`,
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}
						return pcscommand.RunShareCancel(converter.SliceStringToInt64(c.Args()))
					},
				},
			},
		},
		{
			Name:    "offlinedl",
			Aliases: []string{"clouddl", "od"},
			Usage:   "离线下载",
			Description: `支持http/https/ftp/电驴/磁力链协议
	离线下载同时进行的任务数量有限, 超出限制的部分将无法添加.

	示例:

	1. 将百度和腾讯主页, 离线下载到根目录 /
	pant offlinedl add -path=/ http://baidu.com http://qq.com

	2. 添加磁力链接任务
	pant offlinedl add magnet:?xt=urn:btih:xxx

	3. 查询任务ID为 12345 的离线下载任务状态
	pant offlinedl query 12345

	4. 取消任务ID为 12345 的离线下载任务
	pant offlinedl cancel 12345`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				cli.ShowCommandHelp(c, c.Command.Name)
				return nil
			},
			Subcommands: []cli.Command{
				{
					Name:      "add",
					Aliases:   []string{"a"},
					Usage:     "添加离线下载任务",
					UsageText: app.Name + " offlinedl add -path=<离线下载文件保存的路径> 资源地址1 地址2 ...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						return pcscommand.RunCloudDlAddTask(c.Args(), c.String("path"))
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "path",
							Usage: "离线下载文件保存的路径, 默认为工作目录",
						},
					},
				},
				{
					Name:      "query",
					Aliases:   []string{"q"},
					Usage:     "精确查询离线下载任务",
					UsageText: app.Name + " offlinedl query 任务ID1 任务ID2 ...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						taskIDs := converter.SliceStringToInt64(c.Args())
						if len(taskIDs) == 0 {
							return fmt.Errorf("未找到合法的任务ID (task_id)")
						}

						return pcscommand.RunCloudDlQueryTask(taskIDs)
					},
				},
				{
					Name:      "list",
					Aliases:   []string{"ls", "l"},
					Usage:     "查询离线下载任务列表",
					UsageText: app.Name + " offlinedl list",
					Flags:     outputFlags(),
					Action: func(c *cli.Context) error {
						return pcscommand.RunCloudDlListTaskWithOutput(outputMode(c))
					},
				},
				{
					Name:      "cancel",
					Aliases:   []string{"c"},
					Usage:     "取消离线下载任务",
					UsageText: app.Name + " offlinedl cancel 任务ID1 任务ID2 ...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						taskIDs := converter.SliceStringToInt64(c.Args())
						if len(taskIDs) == 0 {
							return fmt.Errorf("未找到合法的任务ID (task_id)")
						}

						return pcscommand.RunCloudDlCancelTask(taskIDs)
					},
				},
				{
					Name:      "delete",
					Aliases:   []string{"del", "d"},
					Usage:     "删除离线下载任务",
					UsageText: app.Name + " offlinedl delete 任务ID1 任务ID2 ...",
					Action: func(c *cli.Context) error {
						isClear := c.Bool("all")
						if c.NArg() < 1 && !isClear {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						if isClear {
							return pcscommand.RunCloudDlClearTask()
						}

						taskIDs := converter.SliceStringToInt64(c.Args())
						if len(taskIDs) == 0 {
							return fmt.Errorf("未找到合法的任务ID (task_id)")
						}

						return pcscommand.RunCloudDlDeleteTask(taskIDs)
					},
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "all",
							Usage: "清空离线下载任务记录, 程序不会进行二次确认, 谨慎操作!!!",
						},
					},
				},
			},
		},
		{
			Name:    "recycle",
			Aliases: []string{"rc"},
			Usage:   "回收站",
			Description: `
	回收站操作.

	示例:

	1. 从回收站还原两个文件, 其中的两个文件的 fs_id 分别为 1013792297798440 和 643596340463870
	pant recycle restore 1013792297798440 643596340463870

	2. 从回收站删除两个文件, 其中的两个文件的 fs_id 分别为 1013792297798440 和 643596340463870
	pant recycle delete 1013792297798440 643596340463870

	3. 清空回收站, 程序不会进行二次确认, 谨慎操作!!!
	pant recycle delete -all
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NumFlags() <= 0 || c.NArg() <= 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
				}
				return nil
			},
			Subcommands: []cli.Command{
				{
					Name:      "list",
					Aliases:   []string{"ls", "l"},
					Usage:     baidupcs.OperationRecycleList,
					UsageText: app.Name + " recycle list",
					Action: func(c *cli.Context) error {
						return pcscommand.RunRecycleListWithOutput(c.Int("page"), outputMode(c))
					},
					Flags: append([]cli.Flag{
						cli.IntFlag{
							Name:  "page",
							Usage: "回收站文件列表页数",
							Value: 1,
						},
					}, outputFlags()...),
				},
				{
					Name:        "restore",
					Aliases:     []string{"r"},
					Usage:       baidupcs.OperationRecycleRestore,
					UsageText:   app.Name + " recycle restore <fs_id 1> <fs_id 2> <fs_id 3> ...",
					Description: `根据文件/目录的 fs_id, 还原回收站指定的文件或目录`,
					Action: func(c *cli.Context) error {
						if c.NArg() <= 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}
						return pcscommand.RunRecycleRestore(c.Args()...)
					},
				},
				{
					Name:        "delete",
					Aliases:     []string{"d"},
					Usage:       baidupcs.OperationRecycleDelete + "/" + baidupcs.OperationRecycleClear,
					UsageText:   app.Name + " recycle delete [-all] <fs_id 1> <fs_id 2> <fs_id 3> ...",
					Description: `根据文件/目录的 fs_id 或 -all 参数, 删除回收站指定的文件或目录或清空回收站`,
					Action: func(c *cli.Context) error {
						if c.Bool("all") {
							return pcscommand.RunRecycleClear()
						}

						if c.NArg() <= 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}
						return pcscommand.RunRecycleDelete(c.Args()...)
					},
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "all",
							Usage: "清空回收站, 程序不会进行二次确认, 谨慎操作!!!",
						},
					},
				},
			},
		},
	}
}
