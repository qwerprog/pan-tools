package pcsutil

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Executable 获取程序所在的真实目录或真实相对路径
func Executable() string {
	executablePath, err := os.Executable()
	if err != nil {
		executablePath, err = filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			executablePath = filepath.Dir(os.Args[0])
		}
	}

	// 读取链接
	linkedExecutablePath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		return executablePath
	}
	return linkedExecutablePath
}

// ExecutablePath 获取程序所在目录
func ExecutablePath() string {
	return filepath.Dir(Executable())
}

// ExecutablePathJoin 返回程序所在目录的子目录
func ExecutablePathJoin(subPath string) string {
	return filepath.Join(ExecutablePath(), subPath)
}

// WalkDir 获取指定目录及所有子目录下的所有文件，可以匹配后缀过滤。
// 支持 Linux/macOS 软链接
func WalkDir(dirPth, suffix string) (files []string, err error) {
	files = make([]string, 0, 32)
	suffix = strings.ToUpper(suffix) //忽略后缀匹配的大小写

	var walkFunc fs.WalkDirFunc
	walkFunc = func(filename string, fi fs.DirEntry, err error) error { //遍历目录
		if err != nil {
			return err
		}
		if fi.IsDir() { // 忽略目录和空文件
			return nil
		}
		fileInfo, err := fi.Info()
		if err != nil || fileInfo.Size() == 0 {
			return nil
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 { // 读取 symbol link
			targetFileInfo, statErr := os.Stat(filename)
			if statErr != nil { // 断开的软链接, 跳过
				return nil
			}
			if targetFileInfo.IsDir() {
				err = filepath.WalkDir(filename+string(os.PathSeparator), walkFunc)
				return err
			}
		}

		if strings.HasSuffix(strings.ToUpper(fi.Name()), suffix) {
			files = append(files, path.Clean(filename))
		}
		return nil
	}

	err = filepath.WalkDir(dirPth, walkFunc)
	return files, err
}

// ConvertToUnixPathSeparator 将 windows 目录分隔符转换为 Unix 的
func ConvertToUnixPathSeparator(p string) string {
	return strings.Replace(p, "\\", "/", -1)
}

func ChPathLegal(p string) bool {
	illegal_chars := "<>|:\"*?,\\"
	if runtime.GOOS == "windows" {
		illegal_chars = "<>|\"*?,\\"
	}

	if strings.ContainsAny(p, illegal_chars) {
		return false
	}
	return true
}

// ExpandLocalPath 清理本地输入路径首尾空白及成对包裹引号，并将 ~ 或 ~/... 展开为用户主目录。
func ExpandLocalPath(p string) string {
	s := strings.TrimSpace(p)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	if s == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return s
	}
	if strings.HasPrefix(s, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(s, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, s[2:])
		}
	}
	return s
}
