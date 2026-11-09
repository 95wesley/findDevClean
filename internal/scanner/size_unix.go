//go:build !windows

package scanner

import (
	"io/fs"
	"syscall"
)

// diskSize is the space the file actually occupies on disk. Sparse files
// (for example VM disk images) report a huge logical size but use little space.
func diskSize(info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Blocks * 512
	}
	return info.Size()
}
