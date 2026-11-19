package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// OutputMode 定义命令结果输出格式（人类可读表格 / JSON / 纯 TSV）。
type OutputMode string

const (
	OutputPretty OutputMode = "pretty"
	OutputJSON   OutputMode = "json"
	OutputPlain  OutputMode = "plain"
)

// ResolveOutputMode 根据命令标志解析输出格式。
func ResolveOutputMode(jsonFlag, plainFlag bool) OutputMode {
	if jsonFlag {
		return OutputJSON
	}
	if plainFlag {
		return OutputPlain
	}
	return OutputPretty
}

// WriteJSON 将结构化数据以缩进清晰的 JSON 格式输出到目标 Writer。
func WriteJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// WriteTSV 将二维字符串行以无转义码、无表头的制表符分隔格式输出（适配 awk/cut/xargs）。
func WriteTSV(w io.Writer, rows [][]string) error {
	for _, row := range rows {
		cleaned := make([]string, len(row))
		for i, cell := range row {
			cleaned[i] = strings.ReplaceAll(strings.ReplaceAll(cell, "\t", " "), "\n", " ")
		}
		if _, err := fmt.Fprintln(w, strings.Join(cleaned, "\t")); err != nil {
			return err
		}
	}
	return nil
}
