package cliapp

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/urfave/cli"
)

var (
	loginMenuItems = []string{
		"扫码登录（推荐，支持百度网盘/百度 App）",
		"粘贴 Cookies 登录",
		"输入 BDUSS + STOKEN 登录",
	}

	isInteractiveLoginTTY = func() bool {
		return ui.IsTTY(os.Stdin) && (ui.IsTTY(os.Stderr) || ui.IsTTY(os.Stdout))
	}
)

func accountCommands(app *cli.App) []cli.Command {
	return []cli.Command{
		{
			Name:  "login",
			Usage: "登录百度账号",
			Description: `
	示例:
		pant login
		pant login -qr
		pant login -cookies="BDUSS=xxxxx; BAIDUID=yyyyyy; STOKEN=zzzzz; ...."
		pant login -bduss=123456789 -stoken=atahsrweoog

	交互式登录:
		直接运行 pant login 将显示交互式登录方式菜单（默认推荐扫码登录，支持百度网盘 App / 百度 App）.
		也可直接运行 pant login -qr (或 --qr / --qrcode) 跳过菜单直接扫码登录.

	百度BDUSS获取方法:
		百度搜索: 获取百度BDUSS
		
	百度Cookies获取办法:
	以Chrome为例，登录到自己的百度网盘主页，F12，然后切换到Network标签，刷新页面，Network标签下会刷出一大堆东西
	找到第一条，点击，看到右侧出现的详情，往下翻到Cookies: xxxx; xxxxx; xxx...这样的字段，从冒号后（没有空格）一直复制到字段末尾`,
			Category: "百度帐号",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				qrMode := c.Bool("qr") || c.Bool("qrcode")
				cookiesSet := c.IsSet("cookies")
				bdussSet := c.IsSet("bduss")

				if (c.IsSet("stoken") || c.IsSet("ptoken")) && !bdussSet {
					return fmt.Errorf("-stoken / -ptoken 需配合 -bduss 参数一起使用（例如: %s login -bduss=<BDUSS> -stoken=<STOKEN>）", app.Name)
				}

				modeCount := 0
				for _, active := range []bool{qrMode, cookiesSet, bdussSet} {
					if active {
						modeCount++
					}
				}
				if modeCount > 1 {
					return fmt.Errorf("请勿同时指定多种登录方式参数（-qr / -cookies / -bduss）")
				}

				if c.NArg() > 0 {
					if modeCount > 0 {
						return fmt.Errorf("login 命令不接受多余的位置参数: %s", c.Args().Get(0))
					}
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				var (
					bduss, ptoken, stoken, cookies string
					err                            error
				)
				switch {
				case qrMode:
					bduss, ptoken, stoken, cookies, err = pcscommand.RunQRLogin()
				case cookiesSet:
					cookies = c.String("cookies")
				case bdussSet:
					bduss = c.String("bduss")
					ptoken = c.String("ptoken")
					stoken = c.String("stoken")
				default:
					if !isInteractiveLoginTTY() {
						return fmt.Errorf("当前为非交互式终端，请指定登录方式参数（例如: %s login -qr、%s login -cookies=<Cookies> 或 %s login -bduss=<BDUSS> -stoken=<STOKEN>）",
							app.Name, app.Name, app.Name)
					}
					menuOut := os.Stderr
					if !ui.IsTTY(os.Stderr) && ui.IsTTY(os.Stdout) {
						menuOut = os.Stdout
					}
					choice, selErr := ui.SelectOption(os.Stdin, menuOut, "选择一种登录方式", loginMenuItems, 0)
					if selErr != nil {
						if errors.Is(selErr, ui.ErrSelectCanceled) {
							return pcscommand.ErrLoginCanceled
						}
						return selErr
					}
					switch choice {
					case 0:
						bduss, ptoken, stoken, cookies, err = pcscommand.RunQRLogin()
					case 1:
						cookies, err = pcscommand.RunCookieLoginPrompt()
					case 2:
						bduss, stoken, err = pcscommand.RunBDUSSLoginPrompt()
					}
				}
				if err != nil {
					return err
				}

				baidu, err := pcsconfig.Config.SetupUserByBDUSS(bduss, ptoken, stoken, cookies)
				if err != nil {
					return err
				}
				if err := pcsconfig.Config.Save(); err != nil {
					return fmt.Errorf("保存配置错误: %w", err)
				}

				if !ui.IsQuiet() {
					ui.PrintSuccess(os.Stdout, "百度帐号登录成功: %s", baidu.Name)
				}
				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "qr, qrcode",
					Usage: "使用百度网盘 / 百度 App 扫码登录 (推荐)",
				},
				cli.StringFlag{
					Name:  "bduss",
					Usage: "使用百度 BDUSS 来登录百度帐号",
				},
				cli.StringFlag{
					Name:  "ptoken",
					Usage: "百度 PTOKEN, 配合 -bduss 参数使用 (可选)",
				},
				cli.StringFlag{
					Name:  "stoken",
					Usage: "百度 STOKEN, 配合 -bduss 参数使用 (可选, 欲使用转存功能则必选)",
				},
				cli.StringFlag{
					Name:  "cookies",
					Usage: "使用百度 Cookies 来登录百度账号",
				},
			},
		},
		{
			Name:  "su",
			Usage: "切换百度帐号",
			Description: `
	切换已登录的百度帐号:
	如果运行该条命令没有提供参数, 程序将会列出所有的百度帐号, 供选择切换.

	示例:
	pant su
	pant su <uid or name>
`,
			Category: "百度帐号",
			Before:   reloadFn,
			After:    saveFunc,
			Action: func(c *cli.Context) error {
				if c.NArg() >= 2 {
					cli.ShowCommandHelp(c, c.Command.Name)
					return nil
				}

				numLogins := pcsconfig.Config.NumLogins()

				if numLogins == 0 {
					return fmt.Errorf("未设置任何百度帐号, 不能切换")
				}

				var (
					inputData = c.Args().Get(0)
					uid       uint64
				)

				if c.NArg() == 1 {
					// 直接切换
					uid, _ = strconv.ParseUint(inputData, 10, 64)
				} else if c.NArg() == 0 {
					// 输出所有帐号供选择切换
					fmt.Println(pcsconfig.Config.BaiduUserList.String())

					// 提示输入 index
					var index string
					fmt.Printf("输入要切换帐号的 # 值 > ")
					_, err := fmt.Scanln(&index)
					if err != nil {
						return nil
					}

					if n, err := strconv.Atoi(index); err == nil && n >= 0 && n < numLogins {
						uid = pcsconfig.Config.BaiduUserList[n].UID
					} else {
						return fmt.Errorf("切换用户失败, 请检查 # 值是否正确")
					}
				} else {
					cli.ShowCommandHelp(c, c.Command.Name)
				}

				switchedUser, err := pcsconfig.Config.SwitchUser(&pcsconfig.BaiduBase{
					Name: inputData,
				})
				if err != nil {
					switchedUser, err = pcsconfig.Config.SwitchUser(&pcsconfig.BaiduBase{
						UID: uid,
					})
					if err != nil {
						return fmt.Errorf("切换用户失败: %w", err)
					}
				}

				if !ui.IsQuiet() {
					ui.PrintSuccess(os.Stdout, "切换用户: %s", switchedUser.Name)
				}
				return nil
			},
		},
		{
			Name:        "logout",
			Usage:       "退出百度帐号",
			Description: "退出当前登录的百度帐号",
			Category:    "百度帐号",
			Before:      reloadFn,
			After:       saveFunc,
			Action: func(c *cli.Context) error {
				if pcsconfig.Config.NumLogins() == 0 {
					ui.PrintWarn(os.Stderr, "未设置任何百度帐号, 不能退出")
					return nil
				}

				var (
					confirm    string
					activeUser = pcsconfig.Config.ActiveUser()
				)

				if !c.Bool("y") {
					fmt.Printf("确认退出百度帐号: %s ? (y/n) > ", activeUser.Name)
					_, err := fmt.Scanln(&confirm)
					if err != nil || (confirm != "y" && confirm != "Y") {
						return nil
					}
				}

				deletedUser, err := pcsconfig.Config.DeleteUser(&pcsconfig.BaiduBase{
					UID: activeUser.UID,
				})
				if err != nil {
					return fmt.Errorf("退出用户 %s 失败: %w", activeUser.Name, err)
				}

				if !ui.IsQuiet() {
					ui.PrintSuccess(os.Stdout, "退出用户成功: %s", deletedUser.Name)
				}
				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "y",
					Usage: "确认退出帐号",
				},
			},
		},
		{
			Name:        "loglist",
			Usage:       "列出帐号列表",
			Description: "列出所有已登录的百度帐号",
			Category:    "百度帐号",
			Before:      reloadFn,
			Flags:       outputFlags(),
			Action: func(c *cli.Context) error {
				users := pcsconfig.Config.BaiduUserList
				switch outputMode(c) {
				case ui.OutputJSON:
					type userSummary struct {
						UID     uint64  `json:"uid"`
						Name    string  `json:"name"`
						Sex     string  `json:"sex"`
						Age     float64 `json:"age"`
						Workdir string  `json:"workdir"`
					}
					list := make([]userSummary, 0, len(users))
					for _, u := range users {
						list = append(list, userSummary{UID: u.UID, Name: u.Name, Sex: u.Sex, Age: u.Age, Workdir: u.Workdir})
					}
					return ui.WriteJSON(os.Stdout, list)
				case ui.OutputPlain:
					rows := make([][]string, 0, len(users))
					for i, u := range users {
						rows = append(rows, []string{strconv.Itoa(i), strconv.FormatUint(u.UID, 10), u.Name, u.Sex, fmt.Sprintf("%.1f", u.Age), u.Workdir})
					}
					return ui.WriteTSV(os.Stdout, rows)
				default:
					if len(users) == 0 {
						if !ui.IsQuiet() {
							ui.PrintInfo(os.Stdout, "当前尚未登录任何百度帐号（运行 '%s login' 登录）", app.Name)
						}
						return nil
					}
					fmt.Print(users.String())
					return nil
				}
			},
		},
		{
			Name:        "who",
			Usage:       "获取当前帐号",
			Description: "获取当前帐号的信息",
			Category:    "百度帐号",
			Before:      reloadFn,
			Flags:       outputFlags(),
			Action: func(c *cli.Context) error {
				activeUser := pcsconfig.Config.ActiveUser()
				switch outputMode(c) {
				case ui.OutputJSON:
					return ui.WriteJSON(os.Stdout, map[string]interface{}{
						"uid":     activeUser.UID,
						"name":    activeUser.Name,
						"sex":     activeUser.Sex,
						"age":     activeUser.Age,
						"workdir": activeUser.Workdir,
					})
				case ui.OutputPlain:
					return ui.WriteTSV(os.Stdout, [][]string{{
						strconv.FormatUint(activeUser.UID, 10),
						activeUser.Name,
						activeUser.Sex,
						fmt.Sprintf("%.1f", activeUser.Age),
						activeUser.Workdir,
					}})
				default:
					colorOn := ui.ColorEnabled(os.Stdout)
					sep := " · "
					if ui.ASCIIOnly() {
						sep = " | "
					}
					if activeUser.Name == "" {
						badge := ui.RenderBadge(os.Stdout, ui.BadgeOffline, "未登录", colorOn)
						fmt.Printf("%s 当前尚未登录百度帐号（运行 '%s login' 登录）\n", badge, app.Name)
						return nil
					}
					badge := ui.RenderBadge(os.Stdout, ui.BadgeActive, "当前", colorOn)
					workdir := activeUser.Workdir
					if workdir == "" {
						workdir = "/"
					}
					fmt.Printf("%s UID: %s%s用户名: %s%s性别: %s%s帐龄: %.1f%s工作目录: %s\n",
						badge, ui.MaskUID(activeUser.UID), sep, activeUser.Name, sep, activeUser.Sex, sep, activeUser.Age, sep, workdir)
					return nil
				}
			},
		},
		{
			Name:        "quota",
			Usage:       "获取网盘配额",
			Description: "获取网盘的总储存空间, 和已使用的储存空间",
			Category:    "百度网盘",
			Before:      reloadFn,
			Flags:       outputFlags(),
			Action: func(c *cli.Context) error {
				return pcscommand.RunGetQuotaWithOutput(outputMode(c))
			},
		},
	}
}
