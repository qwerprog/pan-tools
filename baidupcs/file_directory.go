package baidupcs

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

type (
	// OrderBy 排序字段
	OrderBy string
	// Order 升序降序
	Order string
)

const (
	// OrderByName 根据文件名排序
	OrderByName OrderBy = "name"
	// OrderByTime 根据时间排序
	OrderByTime OrderBy = "time"
	// OrderBySize 根据大小排序, 注意目录无大小
	OrderBySize OrderBy = "size"
	// OrderAsc 升序
	OrderAsc Order = "asc"
	// OrderDesc 降序
	OrderDesc Order = "desc"
)

type (
	// HandleFileDirectoryFunc 处理文件或目录的元信息, 返回值控制是否退出递归
	HandleFileDirectoryFunc func(depth int, fdPath string, fd *FileDirectory, pcsError pcserror.Error) bool

	// FileDirectory 文件或目录的元信息
	FileDirectory struct {
		FsID     int64  // fs_id
		AppID    int64  // app_id
		Path     string // 路径
		Filename string // 文件名 或 目录名
		Ctime    int64  // 创建日期
		Mtime    int64  // 修改日期
		MD5      string // md5 值
		BlockListJSON
		Size        int64  // 文件大小 (目录为0)
		Isdir       bool   // 是否为目录
		Ifhassubdir bool   // 是否含有子目录 (只对目录有效)
		PreBase     string // 真正的base目录

		Parent   *FileDirectory    // 父目录信息
		Children FileDirectoryList // 子目录信息
	}

	// FileDirectoryList FileDirectory 的 指针数组
	FileDirectoryList []*FileDirectory

	// fdJSON 用于解析远程PCS/Pan JSON数据
	fdJSON struct {
		FsID        int64  `json:"fs_id"`           // fs_id
		AppID       int64  `json:"app_id"`          // app_id
		Path        string `json:"path"`            // 路径
		Filename    string `json:"server_filename"` // 文件名 或 目录名
		Ctime       int64  `json:"ctime"`           // 创建日期 (PCS接口)
		Mtime       int64  `json:"mtime"`           // 修改日期 (PCS接口)
		ServerCtime int64  `json:"server_ctime"`    // 创建日期 (Pan接口)
		ServerMtime int64  `json:"server_mtime"`    // 修改日期 (Pan接口)
		MD5         string `json:"md5"`             // md5 值
		BlockListJSON
		Size           int64 `json:"size"` // 文件大小 (目录为0)
		IsdirInt       int8  `json:"isdir"`
		IfhassubdirInt int8  `json:"ifhassubdir"`
	}

	fdDataJSONExport struct {
		*pcserror.PCSErrInfo
		List []*fdJSON `json:"list"`
	}

	// OrderOptions 列文件/目录可选项
	OrderOptions struct {
		By    OrderBy
		Order Order
	}
)

var (
	// DefaultOrderOptions 默认的排序
	DefaultOrderOptions = &OrderOptions{
		By:    OrderByName,
		Order: OrderAsc,
	}
)

// FilesDirectoriesMeta 获取单个文件/目录的元信息
func (pcs *BaiduPCS) FilesDirectoriesMeta(path string) (data *FileDirectory, pcsError pcserror.Error) {
	if path == "" {
		path = PathSeparator
	}

	fds, err := pcs.FilesDirectoriesBatchMeta(path)
	if err != nil {
		return nil, err
	}

	if len(fds) == 0 {
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationFilesDirectoriesMeta,
			ErrType:   pcserror.ErrTypeRemoteError,
			ErrCode:   31066,
			ErrMsg:    "文件或目录不存在",
		}
	}

	// 返回了多条元信息
	if len(fds) != 1 {
		return nil, &pcserror.PCSErrInfo{
			Operation: OperationFilesDirectoriesMeta,
			ErrType:   pcserror.ErrTypeOthers,
			Err:       errors.New("未知返回数据"),
		}
	}
	return fds[0], nil
}

// FilesDirectoriesBatchMeta 获取多个文件/目录的元信息
func (pcs *BaiduPCS) FilesDirectoriesBatchMeta(paths ...string) (data FileDirectoryList, pcsError pcserror.Error) {
	dataReadCloser, pcsError := pcs.PrepareFilesDirectoriesBatchMeta(paths...)
	if pcsError != nil {
		return nil, pcsError
	}

	defer dataReadCloser.Close()

	errInfo := pcserror.NewPCSErrorInfo(OperationFilesDirectoriesMeta)
	// 服务器返回数据进行处理
	jsonData := fdDataJSONExport{
		PCSErrInfo: errInfo,
	}

	pcsError = pcserror.HandleJSONParse(OperationFilesDirectoriesMeta, dataReadCloser, &jsonData)
	if pcsError != nil {
		return
	}

	data = make(FileDirectoryList, 0, len(jsonData.List))
	for _, j := range jsonData.List {
		if j != nil {
			data = append(data, j.toFileDirectory())
		}
	}

	// 修复MD5
	data.fixMD5()
	return
}

// maxListNum 单次拉取目录列表的最大条目数, 网盘 web 列表接口每页上限为 1000
const maxListNum = 1000

// toFileDirectory 将远程接口条目转换为 FileDirectory
func (j *fdJSON) toFileDirectory() *FileDirectory {
	if j == nil {
		return nil
	}
	ctime := j.Ctime
	if ctime == 0 {
		ctime = j.ServerCtime
	}
	mtime := j.Mtime
	if mtime == 0 {
		mtime = j.ServerMtime
	}
	return &FileDirectory{
		FsID:          j.FsID,
		AppID:         j.AppID,
		Path:          j.Path,
		Filename:      j.Filename,
		Ctime:         ctime,
		Mtime:         mtime,
		MD5:           j.MD5,
		BlockListJSON: j.BlockListJSON,
		Size:          j.Size,
		Isdir:         j.IsdirInt == 1,
		Ifhassubdir:   j.IfhassubdirInt == 1,
	}
}

// fdPanListJSON 用于解析 pan.baidu.com/api/list 的整体响应
type fdPanListJSON struct {
	*pcserror.PanErrorInfo
	List []*fdJSON `json:"list"`
}

// FilesDirectoriesList 获取目录下的文件和目录列表
//
// 网盘 web 列表接口单页最多返回 1000 条, 这里按 page 循环分页拉取, 直至取完整个目录
// (修复 https://github.com/qjfoidnh/BaiduPCS-Go/issues/511)。
// 通过 fs_id 去重防御分页异常: 一旦出现重复条目立即终止, 避免死循环与重复数据。
func (pcs *BaiduPCS) FilesDirectoriesList(path string, options *OrderOptions) (data FileDirectoryList, pcsError pcserror.Error) {
	// 防御性上限, 避免接口异常导致死循环
	const maxPage = 2000
	seen := make(map[int64]struct{}, maxListNum)

	for page := 1; page <= maxPage; page++ {
		dataReadCloser, err := pcs.PrepareFilesDirectoriesList(path, options, maxListNum, page)
		if err != nil {
			pcsError = err
			return
		}

		jsonData := fdPanListJSON{
			PanErrorInfo: pcserror.NewPanErrorInfo(OperationFilesDirectoriesList),
		}

		err = pcserror.HandleJSONParse(OperationFilesDirectoriesList, dataReadCloser, &jsonData)
		dataReadCloser.Close()
		if err != nil {
			pcsError = err
			return
		}

		if len(jsonData.List) == 0 {
			break
		}

		// 转换为 FileDirectory 并按 fs_id 去重
		pageList := make(FileDirectoryList, 0, len(jsonData.List))
		for _, j := range jsonData.List {
			if j == nil {
				continue
			}
			if _, ok := seen[j.FsID]; ok {
				// 重复 fs_id 说明分页未继续推进 (或已绕回), 终止遍历
				return
			}
			seen[j.FsID] = struct{}{}
			pageList = append(pageList, j.toFileDirectory())
		}
		// 修复MD5
		pageList.fixMD5()
		data = append(data, pageList...)

		if len(jsonData.List) < maxListNum {
			break // 已取完最后一页
		}
	}
	return
}

// Search 按文件名搜索文件, 不支持查找目录
func (pcs *BaiduPCS) Search(targetPath, keyword string, recursive bool) (fdl FileDirectoryList, pcsError pcserror.Error) {
	if targetPath == "" {
		targetPath = PathSeparator
	}

	dataReadCloser, pcsError := pcs.PrepareSearch(targetPath, keyword, recursive)
	if pcsError != nil {
		return nil, pcsError
	}

	defer dataReadCloser.Close()

	errInfo := pcserror.NewPCSErrorInfo(OperationSearch)
	jsonData := fdDataJSONExport{
		PCSErrInfo: errInfo,
	}

	pcsError = pcserror.HandleJSONParse(OperationSearch, dataReadCloser, &jsonData)
	if pcsError != nil {
		return
	}

	fdl = make(FileDirectoryList, 0, len(jsonData.List))
	for _, j := range jsonData.List {
		if j != nil {
			fdl = append(fdl, j.toFileDirectory())
		}
	}

	// 修复MD5
	fdl.fixMD5()
	return
}

func (pcs *BaiduPCS) recurseList(path string, depth int, options *OrderOptions, prebase string, handleFileDirectoryFunc HandleFileDirectoryFunc) (fdl FileDirectoryList, ok bool) {
	fdl, pcsError := pcs.FilesDirectoriesList(path, options)
	if pcsError != nil {
		ok := handleFileDirectoryFunc(depth, path, nil, pcsError) // 传递错误
		return nil, ok
	}

	for k := range fdl {
		fdl[k].PreBase = prebase
		ok = handleFileDirectoryFunc(depth+1, fdl[k].Path, fdl[k], nil)
		if !ok {
			return
		}

		if !fdl[k].Isdir {
			continue
		}

		fdl[k].Children, ok = pcs.recurseList(fdl[k].Path, depth+1, options, filepath.Join(prebase, filepath.Base(fdl[k].Path)), handleFileDirectoryFunc)
		if !ok {
			return
		}
	}

	return fdl, true
}

// FilesDirectoriesRecurseList 递归获取目录下的文件和目录列表
func (pcs *BaiduPCS) FilesDirectoriesRecurseList(path string, options *OrderOptions, handleFileDirectoryFunc HandleFileDirectoryFunc) (data FileDirectoryList) {
	fd, pcsError := pcs.FilesDirectoriesMeta(path)
	if pcsError != nil {
		handleFileDirectoryFunc(0, path, nil, pcsError) // 传递错误
		return nil
	}

	if !fd.Isdir { // 不是一个目录
		handleFileDirectoryFunc(0, path, fd, nil)
		return FileDirectoryList{fd}
	}
	if !handleFileDirectoryFunc(0, path, fd, nil) {
		return nil
	}

	data, _ = pcs.recurseList(path, 0, options, filepath.Base(path), handleFileDirectoryFunc)
	return data
}

// fixMD5 尝试修复MD5字段
// 服务器返回的MD5字段不一定正确了, 即是BlockList只有一个md5
// MD5字段使用BlockList中的md5
func (f *FileDirectory) fixMD5() {
	if len(f.BlockList) != 1 {
		return
	}
	f.MD5 = f.BlockList[0]
}

func (f *FileDirectory) String() string {
	var builder strings.Builder
	if f.Isdir {
		fmt.Fprintf(&builder, "类型: 目录\n目录路径: %s\n目录名称: %s\n", f.Path, f.Filename)
	} else {
		md5Label := "md5 (截图请打码)"
		if len(f.BlockList) > 1 {
			md5Label = "md5 (可能不正确)"
		}
		fmt.Fprintf(&builder, "类型: 文件\n文件路径: %s\n文件名称: %s\n文件大小: %d, %s\n%s: %s\n",
			f.Path, f.Filename, f.Size, converter.ConvertFileSize(f.Size), md5Label, f.MD5)
	}
	fmt.Fprintf(&builder, "app_id: %d\nfs_id: %d\n创建日期: %s\n修改日期: %s\n",
		f.AppID, f.FsID, pcsutil.FormatTime(f.Ctime), pcsutil.FormatTime(f.Mtime))
	if f.Ifhassubdir {
		builder.WriteString("是否含有子目录: true\n")
	}
	return builder.String()
}

func (fl FileDirectoryList) fixMD5() {
	for _, v := range fl {
		v.fixMD5()
		v.MD5 = DecryptMD5(v.MD5)
	}
}

// TotalSize 获取目录下文件的总大小
func (fl FileDirectoryList) TotalSize() int64 {
	var size int64
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		size += fl[k].Size

		// 递归获取
		if fl[k].Children != nil {
			size += fl[k].Children.TotalSize()
		}
	}
	return size
}

// Count 获取文件总数和目录总数
func (fl FileDirectoryList) Count() (fileN, directoryN int64) {
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		if fl[k].Isdir {
			directoryN++
		} else {
			fileN++
		}

		// 递归获取
		if fl[k].Children != nil {
			fN, dN := fl[k].Children.Count()
			fileN += fN
			directoryN += dN
		}
	}
	return
}

// AllFilePaths 返回所有的网盘路径, 包括子目录
func (fl FileDirectoryList) AllFilePaths() (pcspaths []string) {
	fN, dN := fl.Count()
	pcspaths = make([]string, 0, fN+dN)
	for k := range fl {
		if fl[k] == nil {
			continue
		}

		pcspaths = append(pcspaths, fl[k].Path)

		if fl[k].Children != nil {
			pcspaths = append(pcspaths, fl[k].Children.AllFilePaths()...)
		}
	}
	return
}
