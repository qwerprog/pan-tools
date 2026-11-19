package cliapp

import (
	"fmt"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/tui"
	"github.com/urfave/cli"
)

func fsCommands(app *cli.App) []cli.Command {
	return []cli.Command{
		{
			Name:     "ui",
			Aliases:  []string{"tui", "browse"},
			Category: "百度网盘",
			Usage:    "启动全屏交互式网盘文件管理器 (支持方向键浏览、多选、过滤与传输)",
			Description: `
	pant ui [初始网盘目录]

	启动全屏交互式百度网盘文件浏览器:
	  ↑/↓ 或 j/k 移动光标, ←/→ 或 h/l 进出目录
	  Space 多选文件, / 实时过滤, s 切换排序, r 强制刷新
	  d 下载选中项, u 上传本地路径, n 新建目录, R 重命名, x 移入回收站
	  退出时自动将最后浏览目录同步为当前工作目录.
`,
			Before: reloadFn,
			After:  saveFunc,
			Action: func(c *cli.Context) error {
				return tui.Run(c.Args().Get(0))
			},
		},
		{
			Name:     "cd",
			Category: "百度网盘",
			Usage:    "切换工作目录",
			Description: `
	pant cd <目录, 绝对路径或相对路径>

	示例:

	切换 /我的资源 工作目录:
	pant cd /我的资源

	切换上级目录:
	pant cd ..

	切换根目录:
	pant cd /

	切换 /我的资源 工作目录, 并自动列出 /我的资源 下的文件和目录
	pant cd -l 我的资源

	使用通配符:
	pant cd /我的*
`,
			Before: reloadFn,
			After:  saveFunc,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunChangeDirectory(c.Args().Get(0), c.Bool("l"))
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "l",
					Usage: "切换工作目录后自动列出工作目录下的文件和目录",
				},
			},
		},
		{
			Name:      "ls",
			Aliases:   []string{"l", "ll"},
			Usage:     "列出目录",
			UsageText: app.Name + " ls <目录>",
			Description: `
	列出当前工作目录内的文件和目录, 或指定目录内的文件和目录

	示例:

	列出 我的资源 内的文件和目录
	pant ls 我的资源

	绝对路径
	pant ls /我的资源

	降序排序
	pant ls -desc 我的资源

	按文件大小降序排序
	pant ls -size -desc 我的资源

	使用通配符
	pant ls /我的*
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				orderOptions := &baidupcs.OrderOptions{}
				switch {
				case c.IsSet("asc"):
					orderOptions.Order = baidupcs.OrderAsc
				case c.IsSet("desc"):
					orderOptions.Order = baidupcs.OrderDesc
				default:
					orderOptions.Order = baidupcs.OrderAsc
				}

				switch {
				case c.IsSet("time"):
					orderOptions.By = baidupcs.OrderByTime
				case c.IsSet("name"):
					orderOptions.By = baidupcs.OrderByName
				case c.IsSet("size"):
					orderOptions.By = baidupcs.OrderBySize
				default:
					orderOptions.By = baidupcs.OrderByName
				}

				return pcscommand.RunLs(c.Args().Get(0), &pcscommand.LsOptions{
					Total:  c.Bool("l") || (c.Parent() != nil && c.Parent().Args().Get(0) == "ll"),
					Output: outputMode(c),
				}, orderOptions)
			},
			Flags: append([]cli.Flag{
				cli.BoolFlag{
					Name:  "l",
					Usage: "详细显示",
				},
				cli.BoolFlag{
					Name:  "asc",
					Usage: "升序排序",
				},
				cli.BoolFlag{
					Name:  "desc",
					Usage: "降序排序",
				},
				cli.BoolFlag{
					Name:  "time",
					Usage: "根据时间排序",
				},
				cli.BoolFlag{
					Name:  "name",
					Usage: "根据文件名排序",
				},
				cli.BoolFlag{
					Name:  "size",
					Usage: "根据大小排序",
				},
			}, outputFlags()...),
		},
		{
			Name:      "search",
			Aliases:   []string{"s"},
			Usage:     "搜索文件",
			UsageText: app.Name + " search [-path=<需要检索的目录>] [-r] 关键字",
			Description: `
	按文件名搜索文件（不支持查找目录）。
	默认在当前工作目录搜索.

	示例:

	搜索根目录的文件
	pant search -path=/ 关键字

	搜索当前工作目录的文件
	pant search 关键字

	递归搜索当前工作目录的文件
	pant search -r 关键字
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunSearch(c.String("path"), c.Args().Get(0), &pcscommand.SearchOptions{
					Total:   c.Bool("l"),
					Recurse: c.Bool("r"),
					Output:  outputMode(c),
				})
			},
			Flags: append([]cli.Flag{
				cli.BoolFlag{
					Name:  "l",
					Usage: "详细显示",
				},
				cli.BoolFlag{
					Name:  "r",
					Usage: "递归搜索",
				},
				cli.StringFlag{
					Name:  "path",
					Usage: "需要检索的目录",
					Value: ".",
				},
			}, outputFlags()...),
		},
		{
			Name:      "tree",
			Aliases:   []string{"t"},
			Usage:     "列出目录的树形图",
			UsageText: app.Name + " tree <目录>",
			Description: `
	列出目录树形图。
	默认从当前工作目录开始列出.

	示例:

	从根目录开始列出
	pant tree /

	只列出两层深度
	pant tree --depth 2

	同时显示文件名和fsid
	pant tree --fsid
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				return pcscommand.RunTree(c.Args().Get(0), 0, &pcscommand.TreeOptions{
					Depth:    c.Int("depth"),
					ShowFsid: c.Bool("fsid"),
				})
			},
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "depth",
					Usage: "显示深度",
					Value: -1,
				},
				cli.BoolFlag{
					Name:  "fsid",
					Usage: "带fsid显示",
				},
			},
		},
		{
			Name:      "pwd",
			Usage:     "输出工作目录",
			UsageText: app.Name + " pwd",
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				fmt.Println(pcsconfig.Config.ActiveUser().Workdir)
				return nil
			},
		},
		{
			Name:        "meta",
			Usage:       "获取文件/目录的元信息",
			UsageText:   app.Name + " meta <文件/目录1> <文件/目录2> <文件/目录3> ...",
			Description: "默认获取工作目录元信息",
			Category:    "百度网盘",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				var (
					ca = c.Args()
					as []string
				)
				if len(ca) == 0 {
					as = []string{""}
				} else {
					as = ca
				}

				return pcscommand.RunGetMeta(as...)
			},
		},
		{
			Name:      "rm",
			Usage:     "删除文件/目录",
			UsageText: app.Name + " rm <文件/目录的路径1> <文件/目录2> <文件/目录3> ...",
			Description: `
	注意: 删除多个文件和目录时, 请确保每一个文件和目录都存在, 否则删除操作会失败.
	被删除的文件或目录可在网盘文件回收站找回.

	示例:

	删除 /我的资源/1.mp4
	pant rm /我的资源/1.mp4

	删除 /我的资源/1.mp4 和 /我的资源/2.mp4
	pant rm /我的资源/1.mp4 /我的资源/2.mp4

	删除 /我的资源 内的所有文件和目录, 但不删除该目录
	pant rm /我的资源/*

	删除 /我的资源 整个目录 !!
	pant rm /我的资源
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunRemove(c.Args()...)
			},
		},
		{
			Name:      "mkdir",
			Usage:     "创建目录",
			UsageText: app.Name + " mkdir <目录>",
			Category:  "百度网盘",
			Before:    reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunMkdir(c.Args().Get(0))
			},
		},
		{
			Name:  "cp",
			Usage: "拷贝文件/目录",
			UsageText: `pant cp <文件/目录> <目标文件/目录>
	pant cp <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>`,
			Description: `
	注意: 拷贝多个文件和目录时, 请确保每一个文件和目录都存在, 否则拷贝操作会失败.

	示例:

	将 /我的资源/1.mp4 复制到 根目录 /
	pant cp /我的资源/1.mp4 /

	将 /我的资源/1.mp4 和 /我的资源/2.mp4 复制到 根目录 /
	pant cp /我的资源/1.mp4 /我的资源/2.mp4 /
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunCopy(c.Args()...)
			},
		},
		{
			Name:  "mv",
			Usage: "移动/重命名文件/目录",
			UsageText: `移动:
	pant mv <文件/目录1> <文件/目录2> <文件/目录3> ... <目标目录>

	重命名:
	pant mv <文件/目录> <重命名的文件/目录>`,
			Description: `
	注意: 移动多个文件和目录时, 请确保每一个文件和目录都存在, 否则移动操作会失败.

	示例:

	将 /我的资源/1.mp4 移动到 根目录 /
	pant mv /我的资源/1.mp4 /

	将 /我的资源/1.mp4 重命名为 /我的资源/3.mp4
	pant mv /我的资源/1.mp4 /我的资源/3.mp4
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() <= 1 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunMove(c.Args()...)
			},
		},
		{
			Name:      "rename",
			Aliases:   []string{"rn"},
			Usage:     "重命名文件/目录",
			UsageText: app.Name + " rename <原文件/目录> <新文件名或目标路径>",
			Description: `
	重命名单个文件或目录, 新名称可以直接填写文件名(自动保持在同一目录下)或完整路径.

	示例:

	将当前目录或指定目录下的 1.mp4 重命名为 2.mp4:
	pant rename /我的资源/1.mp4 2.mp4
`,
			Category: "百度网盘",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				if c.NArg() != 2 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				return pcscommand.RunRename(c.Args().Get(0), c.Args().Get(1))
			},
		},
	}
}
