// Package pcsliner 命令行 readline 工具包
package pcsliner

import (
	"os"

	"github.com/mattn/go-runewidth"
	"github.com/peterh/liner"
)

func init() {
	if os.Getenv("RUNEWIDTH_EASTASIAN") != "1" {
		runewidth.EastAsianWidth = false
		runewidth.DefaultCondition.EastAsianWidth = false
	}
}

// PCSLiner 封装 *liner.State, 提供更简便的操作
type PCSLiner struct {
	State   *liner.State
	History *LineHistory

	tmode liner.ModeApplier
	lmode liner.ModeApplier

	paused bool
}

// NewLiner 返回 *PCSLiner, 默认设置允许 Ctrl+C 结束
func NewLiner() *PCSLiner {
	pl := &PCSLiner{}
	pl.tmode, _ = liner.TerminalMode()

	line := liner.NewLiner()
	pl.lmode, _ = liner.TerminalMode()

	line.SetMultiLineMode(true)
	line.SetCtrlCAborts(true)

	pl.State = line

	return pl
}

// Pause 暂停服务
func (pl *PCSLiner) Pause() error {
	if pl == nil || pl.paused {
		return nil
	}

	pl.paused = true
	_ = pl.DoWriteHistory()

	if pl.tmode == nil {
		return nil
	}
	return pl.tmode.ApplyMode()
}

// Resume 恢复服务
func (pl *PCSLiner) Resume() error {
	if pl == nil || !pl.paused {
		return nil
	}

	pl.paused = false

	if pl.lmode == nil {
		return nil
	}
	return pl.lmode.ApplyMode()
}

// Close 关闭服务
func (pl *PCSLiner) Close() (err error) {
	if pl == nil {
		return nil
	}
	if pl.State != nil {
		err = pl.State.Close()
		if err != nil {
			return err
		}
	}

	if pl.History != nil && pl.History.historyFile != nil {
		closeErr := pl.History.historyFile.Close()
		pl.History.historyFile = nil
		return closeErr
	}

	return nil
}
