//go:build windows

package cfhosts

import "os"

// chownLike 在 Windows 上什么都不做：Windows 没有 uid/gid 概念，属主由 ACL 决定，
// os.Chown 在该平台也不生效。保留同名函数只是为了让上层代码保持单一路径。
func chownLike(path string, info os.FileInfo) error {
	return nil
}
