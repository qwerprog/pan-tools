package args_test

import (
	"reflect"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner/args"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "quotes_escapes_and_empty_arg",
			line: `  one two three "double quotes" 'single quotes'  ""   arg\ with\ spaces "\"quotes\" in 'quotes'"`,
			want: []string{
				"one",
				"two",
				"three",
				"double quotes",
				"single quotes",
				"",
				"arg with spaces",
				`"quotes" in 'quotes'`,
			},
		},
		{
			name: "cjk_filename",
			line: ` cd  英语_800个有趣句子帮你记忆7000个单词_42页.doc`,
			want: []string{"cd", "英语_800个有趣句子帮你记忆7000个单词_42页.doc"},
		},
		{
			name: "whitespace_only",
			line: "   \t  ",
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := args.Parse(tc.line)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args.Parse(%q) = %#v, want %#v", tc.line, got, tc.want)
			}
		})
	}
}
