//go:build windows

package scanner

import "io/fs"

// diskSize falls back to the logical size on Windows, where the allocated
// size is not available through the standard library.
func diskSize(info fs.FileInfo) int64 {
	return info.Size()
}
