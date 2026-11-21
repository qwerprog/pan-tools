package pcscommand

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

// RunCloudDlAddTask 执行添加离线下载任务
func RunCloudDlAddTask(sourceURLs []string, savePath string) error {
	var (
		err      error
		firstErr error
		errItems []string
		pcs      = GetBaiduPCS()
	)
	err = matchPathByShellPatternOnce(&savePath)
	if err != nil {
		return err
	}

	var taskid int64
	for k := range sourceURLs {
		taskid, err = pcs.CloudDlAddTask(sourceURLs[k], savePath+baidupcs.PathSeparator)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			errItems = append(errItems, fmt.Sprintf("[%d] %s (%s)", k+1, sourceURLs[k], err))
			continue
		}

		if ui.IsQuiet() {
			fmt.Println(taskid)
		} else {
			ui.PrintSuccess(os.Stdout, "[%d] 添加离线任务成功 · task_id: %d · 源地址: %s · 保存路径: %s", k+1, taskid, sourceURLs[k], savePath)
		}
	}
	if len(errItems) == 1 && len(sourceURLs) == 1 {
		return fmt.Errorf("添加离线任务失败 (%s): %w", sourceURLs[0], firstErr)
	}
	if len(errItems) > 0 {
		return fmt.Errorf("添加离线任务失败 (%d/%d): %s (%w)", len(errItems), len(sourceURLs), strings.Join(errItems, "; "), firstErr)
	}
	return nil
}

// RunCloudDlQueryTask 精确查询离线下载任务
func RunCloudDlQueryTask(taskIDs []int64) error {
	cl, err := GetBaiduPCS().CloudDlQueryTask(taskIDs)
	if err != nil {
		return err
	}

	fmt.Print(pcsfunctions.FormatCloudDlTaskList(cl))
	return nil
}

// RunCloudDlListTaskWithOutput 查询离线下载任务列表并支持多种输出模式。
func RunCloudDlListTaskWithOutput(mode ui.OutputMode) error {
	cl, err := GetBaiduPCS().CloudDlListTask()
	if err != nil {
		return err
	}

	switch mode {
	case ui.OutputJSON:
		return ui.WriteJSON(os.Stdout, cl)
	case ui.OutputPlain:
		rows := make([][]string, 0, len(cl))
		for _, v := range cl {
			rows = append(rows, []string{
				strconv.FormatInt(v.TaskID, 10),
				v.TaskName,
				strconv.FormatInt(v.FileSize, 10),
				path.Clean(v.SavePath),
				v.StatusText,
				v.SourceURL,
			})
		}
		return ui.WriteTSV(os.Stdout, rows)
	default:
		fmt.Print(pcsfunctions.FormatCloudDlTaskList(cl))
		return nil
	}
}

// RunCloudDlCancelTask 取消离线下载任务
func RunCloudDlCancelTask(taskIDs []int64) error {
	var (
		firstErr error
		errItems []string
	)
	for _, id := range taskIDs {
		err := GetBaiduPCS().CloudDlCancelTask(id)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			errItems = append(errItems, fmt.Sprintf("[%d] %s", id, err))
			continue
		}

		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "[%d] 取消离线任务成功", id)
		}
	}
	if len(errItems) == 1 && len(taskIDs) == 1 {
		return fmt.Errorf("[%d] %w", taskIDs[0], firstErr)
	}
	if len(errItems) > 0 {
		return fmt.Errorf("取消离线任务失败 (%d/%d): %s (%w)", len(errItems), len(taskIDs), strings.Join(errItems, "; "), firstErr)
	}
	return nil
}

// RunCloudDlDeleteTask 删除离线下载任务
func RunCloudDlDeleteTask(taskIDs []int64) error {
	var (
		firstErr error
		errItems []string
	)
	for _, id := range taskIDs {
		err := GetBaiduPCS().CloudDlDeleteTask(id)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			errItems = append(errItems, fmt.Sprintf("[%d] %s", id, err))
			continue
		}

		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "[%d] 删除离线任务成功", id)
		}
	}
	if len(errItems) == 1 && len(taskIDs) == 1 {
		return fmt.Errorf("[%d] %w", taskIDs[0], firstErr)
	}
	if len(errItems) > 0 {
		return fmt.Errorf("删除离线任务失败 (%d/%d): %s (%w)", len(errItems), len(taskIDs), strings.Join(errItems, "; "), firstErr)
	}
	return nil
}

// RunCloudDlClearTask 清空离线下载任务记录
func RunCloudDlClearTask() error {
	total, err := GetBaiduPCS().CloudDlClearTask()
	if err != nil {
		return err
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "%s成功 (共清除 %d 条记录)", baidupcs.OperationCloudDlClearTask, total)
	}
	return nil
}
