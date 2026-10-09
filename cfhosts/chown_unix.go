//go:build !windows

package cfhosts

import (
	"os"
	"syscall"
)

// chownLike 把 path 的属主设成与 info 一致，让原子替换后的文件保持原属主。
// 只在 root 下执行：非 root 无权 Chown，而原文件本来就属于当前用户。
// 这段逻辑单独成文件，是因为 syscall.Stat_t 在 Windows 上不存在 ——
// 写在一起会让 GOOS=windows 的交叉编译直接失败。
func chownLike(path string, info os.FileInfo) error {
	if os.Geteuid() != 0 {
		return nil
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(path, int(st.Uid), int(st.Gid))
}
