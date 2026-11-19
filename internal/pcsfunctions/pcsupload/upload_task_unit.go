package pcsupload

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

type (
	// StepUpload 上传步骤
	StepUpload int

	// UploadTaskUnit 上传的任务单元
	UploadTaskUnit struct {
		LocalFileChecksum *checksum.LocalFileChecksum // 要上传的本地文件详情
		Step              StepUpload
		SavePath          string // 保存路径
		PrintFormat       string

		PCS               *baidupcs.BaiduPCS
		PCSClient         *requester.HTTPClient
		UploadingDatabase *UploadingDatabase // 数据库
		Parallel          int
		MaxRate           int64  // 最大上传限速 (字节/秒, <=0 不限速)
		NoRapidUpload     bool   // 禁用秒传
		NoSplitFile       bool   // 禁用分片上传
		Policy            string // 上传重名文件策略

		UploadStatistic *UploadStatistic
		Dashboard       *ui.TransferDashboard

		taskInfo *taskframework.TaskInfo
		panDir   string
		panFile  string
		state    *uploader.InstanceState
		// prepareResult 区分策略正常跳过与准备阶段真实失败。
		prepareResult *taskframework.TaskUnitRunResult
	}
)

const (
	// StepUploadInit 初始化步骤
	StepUploadInit StepUpload = iota
	// StepUploadRapidUpload 秒传步骤
	StepUploadRapidUpload
	// StepUploadUpload 正常上传步骤
	StepUploadUpload
	// JustGoon 跳过上传
	JustGoon
)

const (
	StrUploadFailed    = "上传文件失败"
	DefaultPrintFormat = "\r[%s] ↑ %s/%s %s/s in %s ............"
	DefaultContentSize = 4 * converter.KB
)

func (utu *UploadTaskUnit) SetTaskInfo(taskInfo *taskframework.TaskInfo) {
	utu.taskInfo = taskInfo
}

func (utu *UploadTaskUnit) dashboardActive() bool {
	return utu.Dashboard != nil && utu.Dashboard.Enabled()
}

func (utu *UploadTaskUnit) formatDoneSummary(tag string) string {
	r := ui.NewRenderer(os.Stdout, nil)
	icon := "✓"
	if ui.ASCIIOnly() {
		icon = "[OK]"
	}
	okBadge := r.NewStyle().Foreground(ui.Palette.Success).Bold(true).Render(icon)
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	name := r.NewStyle().Foreground(ui.Palette.VaultCore).Render(filepath.Base(utu.LocalFileChecksum.Path))
	sizeStr := converter.ConvertFileSize(utu.LocalFileChecksum.Length, 2)
	return fmt.Sprintf("%s %s %s  %s",
		okBadge,
		muted.Render("["+utu.taskInfo.Id()+"]"),
		name,
		muted.Render(sizeStr+" · "+tag+" -> "+utu.SavePath),
	)
}

func (utu *UploadTaskUnit) completeSkip(summaryTag, message string, result *taskframework.TaskUnitRunResult) {
	if utu.UploadingDatabase != nil && utu.LocalFileChecksum != nil {
		if utu.UploadingDatabase.Delete(&utu.LocalFileChecksum.LocalFileMeta) {
			_ = utu.UploadingDatabase.Save()
		}
	}
	skipLine := utu.formatDoneSummary(summaryTag)
	if utu.Dashboard != nil {
		utu.Dashboard.CompleteSlot(utu.taskInfo.Id(), skipLine)
	} else {
		ui.Statusln(skipLine)
	}
	result.Succeed = true
	result.NeedRetry = false
	result.Extra = baidupcs.SkipPolicy
	result.Err = nil
	result.ResultMessage = message
}

// prepareFile 解析文件与断点状态，决定后续执行阶段
func (utu *UploadTaskUnit) prepareFile() {
	panDir, panFile := path.Split(utu.SavePath)
	utu.panDir = path.Clean(panDir)
	utu.panFile = panFile

	utu.state = utu.UploadingDatabase.Search(&utu.LocalFileChecksum.LocalFileMeta)
	if utu.state != nil || utu.LocalFileChecksum.LocalFileMeta.BlocksList != nil {
		if !utu.dashboardActive() {
			ui.Statusf("[%s] 检测到断点记录，继续上传 %s\n", utu.taskInfo.Id(), panFile)
		}
		if utu.LocalFileChecksum.LocalFileMeta.BlocksList != nil {
			utu.Step = StepUploadRapidUpload
		} else {
			utu.Step = StepUploadUpload
		}
		return
	}
	utu.state = &uploader.InstanceState{}

	if utu.LocalFileChecksum.Length >= baidupcs.RecommendedUploadSize {
		ui.PrintWarn(os.Stderr, "[%s] 文件超过 32GB，建议切分后上传", utu.taskInfo.Id())
	}

	if utu.LocalFileChecksum.Length > baidupcs.MinCheckLeftSpaceThreshold {
		freeSpace, err := utu.PCS.SpaceLeftInfo()
		if err == nil && freeSpace < utu.LocalFileChecksum.Length {
			utu.prepareResult = &taskframework.TaskUnitRunResult{
				ResultMessage: "目标文件大小超过剩余空间",
				Err:           errors.New("目标文件大小超过百度网盘剩余空间"),
			}
			utu.Step = JustGoon
			return
		}
	}

	if utu.NoRapidUpload {
		info, pcsError := utu.PCS.FakeRapidUpload(utu.SavePath, utu.Policy, utu.LocalFileChecksum.Length)
		if pcsError != nil {
			errcode := pcsError.GetRemoteErrCode()
			if errcode != baidupcs.ErrCodeSkipExisting && errcode != baidupcs.ErrCodeRsyncSameSize {
				utu.Step = StepUploadRapidUpload
				return
			}
			res := &taskframework.TaskUnitRunResult{}
			if errcode == baidupcs.ErrCodeRsyncSameSize {
				utu.completeSkip("目标大小未发生改变, 跳过", fmt.Sprintf("%s 目标大小未发生改变, 跳过", utu.SavePath), res)
			} else {
				utu.completeSkip("远端已存在, 跳过", fmt.Sprintf("%s 目标文件已存在, 跳过", utu.SavePath), res)
			}
			utu.prepareResult = res
			utu.Step = JustGoon
			return
		}
		utu.state = &uploader.InstanceState{
			Uploadid: info.UploadID,
		}
		utu.UploadingDatabase.Save()
		utu.Step = StepUploadUpload
		return
	}

	utu.Step = StepUploadRapidUpload
}

func (utu *UploadTaskUnit) OnRetry(lastRunResult *taskframework.TaskUnitRunResult) {
	var msg string
	if lastRunResult.Err == nil {
		msg = fmt.Sprintf("[%s] %s, 重试 %d/%d", utu.taskInfo.Id(), lastRunResult.ResultMessage, utu.taskInfo.Retry(), utu.taskInfo.MaxRetry())
	} else {
		msg = fmt.Sprintf("[%s] %s, %s, 重试 %d/%d", utu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err, utu.taskInfo.Retry(), utu.taskInfo.MaxRetry())
	}
	if utu.Dashboard != nil {
		utu.Dashboard.ClearSlot(utu.taskInfo.Id(), msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (utu *UploadTaskUnit) OnSuccess(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (utu *UploadTaskUnit) OnFailed(lastRunResult *taskframework.TaskUnitRunResult) {
	var msg string
	if lastRunResult.Err == nil {
		msg = fmt.Sprintf("[%s] %s", utu.taskInfo.Id(), lastRunResult.ResultMessage)
	} else {
		msg = fmt.Sprintf("[%s] %s, %s", utu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err)
	}
	if utu.Dashboard != nil {
		utu.Dashboard.ClearSlot(utu.taskInfo.Id(), msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (utu *UploadTaskUnit) OnComplete(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (utu *UploadTaskUnit) RetryWait() time.Duration {
	return pcsfunctions.RetryWait(utu.taskInfo.Retry())
}

func (utu *UploadTaskUnit) runPreparedStep() *taskframework.TaskUnitRunResult {
	if utu.prepareResult != nil {
		return utu.prepareResult
	}
	return &taskframework.TaskUnitRunResult{
		ResultMessage: StrUploadFailed,
		Err:           errors.New("上传准备阶段未生成结果"),
	}
}

func (utu *UploadTaskUnit) Run() *taskframework.TaskUnitRunResult {
	if utu.LocalFileChecksum.Length > baidupcs.MaxUploadSize {
		return &taskframework.TaskUnitRunResult{
			ResultMessage: "文件大小超过128G, 无法上传",
			Err:           fmt.Errorf("文件 %s 大小超过128G", utu.LocalFileChecksum.Path),
		}
	}

	if err := utu.LocalFileChecksum.OpenPath(); err != nil {
		return &taskframework.TaskUnitRunResult{
			ResultMessage: "文件不可读",
			Err:           err,
		}
	}
	defer utu.LocalFileChecksum.Close()

	utu.prepareFile()

	switch utu.Step {
	case JustGoon:
		return utu.runPreparedStep()
	case StepUploadRapidUpload:
		isContinue, rapidResult := utu.rapidUpload()
		if !isContinue {
			return rapidResult
		}
		return utu.upload()
	case StepUploadUpload:
		return utu.upload()
	default:
		return utu.runPreparedStep()
	}
}
