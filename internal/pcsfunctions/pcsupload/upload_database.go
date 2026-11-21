package pcsupload

import (
	"errors"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type (
	// Uploading 未完成上传的信息
	Uploading struct {
		*checksum.LocalFileMeta
		State *uploader.InstanceState `json:"state"`
	}

	// UploadingDatabase 未完成上传的数据库
	UploadingDatabase struct {
		lock          sync.RWMutex
		UploadingList []*Uploading `json:"upload_state"`
		Timestamp     int64        `json:"timestamp"`

		dataFile *os.File
	}
)

// NewUploadingDatabase 初始化未完成上传的数据库, 从库中读取内容
func NewUploadingDatabase() (ud *UploadingDatabase, err error) {
	file, err := os.OpenFile(filepath.Join(pcsconfig.GetConfigDir(), UploadingFileName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = file.Chmod(0600)

	ud = &UploadingDatabase{
		dataFile: file,
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	if info.Size() <= 0 {
		return ud, nil
	}

	err = pcsutil.UnmarshalData(file, ud)
	if err != nil {
		if truncErr := file.Truncate(0); truncErr != nil {
			_ = file.Close()
			return nil, truncErr
		}
		if _, seekErr := file.Seek(0, os.SEEK_SET); seekErr != nil {
			_ = file.Close()
			return nil, seekErr
		}
		ud.UploadingList = nil
		ud.Timestamp = 0
	}

	return ud, nil
}

// Save 保存内容
func (ud *UploadingDatabase) Save() error {
	if ud.dataFile == nil {
		return errors.New("dataFile is nil")
	}

	ud.lock.Lock()
	defer ud.lock.Unlock()

	ud.Timestamp = time.Now().Unix()

	var (
		builder = &strings.Builder{}
		err     = pcsutil.MarshalData(builder, ud)
	)
	if err != nil {
		return err
	}

	err = ud.dataFile.Truncate(int64(builder.Len()))
	if err != nil {
		return err
	}

	str := builder.String()
	_, err = ud.dataFile.WriteAt(converter.ToBytes(str), 0)
	if err != nil {
		return err
	}

	return nil
}

// UpdateUploading 更新正在上传
func (ud *UploadingDatabase) UpdateUploading(meta *checksum.LocalFileMeta, state *uploader.InstanceState) {
	ud.lock.Lock()
	defer ud.lock.Unlock()
	if meta == nil {
		return
	}
	meta.CompleteAbsPath()
	for k, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.LocalFileMeta.EqualLengthMD5(meta) || uploading.LocalFileMeta.Path == meta.Path {
			ud.UploadingList[k].State = state
			return
		}
	}

	ud.UploadingList = append(ud.UploadingList, &Uploading{
		LocalFileMeta: meta,
		State:         state,
	})
}

func (ud *UploadingDatabase) deleteIndex(k int) {
	ud.UploadingList = append(ud.UploadingList[:k], ud.UploadingList[k+1:]...)
}

// Delete 删除
func (ud *UploadingDatabase) Delete(meta *checksum.LocalFileMeta) bool {
	ud.lock.Lock()
	defer ud.lock.Unlock()
	return ud.deleteLocked(meta)
}

func (ud *UploadingDatabase) deleteLocked(meta *checksum.LocalFileMeta) bool {
	if meta == nil {
		return false
	}
	meta.CompleteAbsPath()
	for k, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.LocalFileMeta.EqualLengthMD5(meta) || uploading.LocalFileMeta.Path == meta.Path {
			ud.deleteIndex(k)
			return true
		}
	}
	return false
}

// Search 搜索
func (ud *UploadingDatabase) Search(meta *checksum.LocalFileMeta) *uploader.InstanceState {
	if meta == nil {
		return nil
	}

	meta.CompleteAbsPath()
	ud.lock.Lock()
	defer ud.lock.Unlock()

	ud.clearModTimeChangeLocked()
	for _, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.BlocksList == nil && (uploading.State == nil || uploading.State.Uploadid == "") {
			continue
		}
		if uploading.LocalFileMeta.MD5 != nil && uploading.LocalFileMeta.EqualLengthMD5(meta) {
			return uploading.State
		}
		if uploading.LocalFileMeta.Path == meta.Path {
			// 移除旧的信息
			// 目前只是比较了文件大小
			if meta.Length != uploading.LocalFileMeta.Length {
				ud.deleteLocked(meta)
				return nil
			}

			meta.MD5 = uploading.LocalFileMeta.MD5
			meta.SliceMD5 = uploading.LocalFileMeta.SliceMD5
			meta.BlocksList = uploading.LocalFileMeta.BlocksList
			return uploading.State
		}
	}
	return nil
}

func (ud *UploadingDatabase) clearModTimeChangeLocked() {
	for i := 0; i < len(ud.UploadingList); i++ {
		uploading := ud.UploadingList[i]
		if uploading.LocalFileMeta == nil {
			continue
		}

		if uploading.ModTime == -1 { // 忽略
			continue
		}

		info, err := os.Stat(uploading.LocalFileMeta.Path)
		if err != nil {
			ud.deleteIndex(i)
			i--
			pcsUploadVerbose.Warnf("clear invalid file path: %s, err: %s\n", uploading.LocalFileMeta.Path, err)
			continue
		}

		if uploading.LocalFileMeta.ModTime != info.ModTime().Unix() {
			ud.deleteIndex(i)
			i--
			pcsUploadVerbose.Infof("clear modified file path: %s\n", uploading.LocalFileMeta.Path)
			continue
		}
	}
}

// Close 关闭数据库
func (ud *UploadingDatabase) Close() error {
	if ud == nil || ud.dataFile == nil {
		return nil
	}
	return ud.dataFile.Close()
}
