package ui

import (
	"bytes"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli"
)

const (
	appHelpTemplate = `{{section "BRAND"}}
{{logo}}

{{section "NAME"}}
  {{cmd .Name}}{{if .Usage}} - {{.Usage}}{{end}}

{{section "USAGE"}}
  {{if .UsageText}}{{.UsageText}}{{else}}{{cmd .HelpName}} {{if .VisibleFlags}}{{muted "[global options]"}}{{end}}{{if .Commands}} {{accent "command"}} {{muted "[command options]"}}{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}{{muted "[arguments...]"}}{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

{{section "VERSION"}}
  {{accent .Version}}{{end}}{{end}}{{if .Description}}

{{section "DESCRIPTION"}}
  {{.Description}}{{end}}{{if .VisibleCommands}}

{{section "COMMANDS"}}{{range .VisibleCategories}}{{if .Name}}
  {{cat .Name}}{{end}}{{range .VisibleCommands}}
    {{fmtCmd .Names}}  {{.Usage}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

{{section "GLOBAL OPTIONS"}}
  {{range $index, $option := .VisibleFlags}}{{if $index}}
  {{end}}{{$option}}{{end}}{{end}}{{if .Copyright}}

{{section "COPYRIGHT"}}
  {{muted .Copyright}}{{end}}
`

	commandHelpTemplate = `{{section "COMMAND"}}
  {{cmd .HelpName}}{{if .Usage}} - {{.Usage}}{{end}}

{{section "USAGE"}}
  {{if .UsageText}}{{.UsageText}}{{else}}{{cmd .HelpName}}{{if .VisibleFlags}} {{muted "[command options]"}}{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}{{muted "[arguments...]"}}{{end}}{{end}}{{if .Category}}

{{section "CATEGORY"}}
  {{cat .Category}}{{end}}{{if .Description}}

{{section "DESCRIPTION"}}
  {{.Description}}{{end}}{{if .VisibleFlags}}

{{section "OPTIONS"}}
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
`

	subcommandHelpTemplate = `{{section "SUBCOMMAND"}}
  {{cmd .HelpName}}{{if .Usage}} - {{.Usage}}{{end}}

{{section "USAGE"}}
  {{if .UsageText}}{{.UsageText}}{{else}}{{cmd .HelpName}} command{{if .VisibleFlags}} {{muted "[command options]"}}{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}{{muted "[arguments...]"}}{{end}}{{end}}{{if .Description}}

{{section "DESCRIPTION"}}
  {{.Description}}{{end}}

{{section "COMMANDS"}}{{range .VisibleCategories}}{{if .Name}}
  {{cat .Name}}{{end}}{{range .VisibleCommands}}
    {{fmtCmd .Names}}  {{.Usage}}{{end}}
{{end}}{{if .VisibleFlags}}
{{section "OPTIONS"}}
  {{range .VisibleFlags}}{{.}}
  {{end}}{{end}}
`
)

// InstallHelpTemplates 将 pan-tools Candidate 3 风格的 Help 模板与按终端能力着色的 HelpPrinter 注册到 urfave/cli。
func InstallHelpTemplates() {
	cli.AppHelpTemplate = appHelpTemplate
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.SubcommandHelpTemplate = subcommandHelpTemplate
	cli.HelpPrinterCustom = renderCLIHelp
	cli.HelpPrinter = func(w io.Writer, templ string, data interface{}) {
		renderCLIHelp(w, templ, data, nil)
	}
}

func renderCLIHelp(out io.Writer, templ string, data interface{}, customFuncs map[string]interface{}) {
	colorOn := ColorEnabled(out)
	asciiOn := ASCIIOnly()
	r := NewRenderer(out, &colorOn)

	sectionStyle := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	catStyle := r.NewStyle().Foreground(Palette.Secondary).Bold(true)
	cmdStyle := r.NewStyle().Foreground(Palette.Accent).Bold(true)
	mutedStyle := r.NewStyle().Foreground(Palette.Muted)

	logoIndented := indentBlock(RenderLogo(out, colorOn, asciiOn), "  ")

	funcMap := template.FuncMap{
		"join": strings.Join,
		"logo": func() string { return logoIndented },
		"section": func(s string) string {
			return sectionStyle.Render(s + ":")
		},
		"cat": func(s string) string {
			return catStyle.Render("[" + s + "]")
		},
		"cmd": func(s string) string {
			return cmdStyle.Render(s)
		},
		"accent": func(s string) string {
			return cmdStyle.Render(s)
		},
		"muted": func(s string) string {
			return mutedStyle.Render(s)
		},
		"fmtCmd": func(names []string) string {
			joined := strings.Join(names, ", ")
			pad := 24 - lipgloss.Width(joined)
			if pad < 1 {
				pad = 1
			}
			return cmdStyle.Render(joined) + strings.Repeat(" ", pad)
		},
	}
	for k, v := range customFuncs {
		funcMap[k] = v
	}

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 1, 8, 2, ' ', 0)
	t := template.Must(template.New("help").Funcs(funcMap).Parse(templ))
	if err := t.Execute(tw, data); err != nil {
		return
	}
	_ = tw.Flush()
	_, _ = out.Write(buf.Bytes())
}
