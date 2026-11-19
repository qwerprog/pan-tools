//go:build windows || plan9 || aix
// +build windows plan9 aix

package pcsutil

func AccessRDWR(path string) bool {
	return true
}
