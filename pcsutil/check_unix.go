//go:build !windows && !plan9 && !aix
// +build !windows,!plan9,!aix

package pcsutil

import (
	"syscall"
)

func AccessRDWR(path string) bool {
	return syscall.Access(path, syscall.O_RDWR) == nil
}
