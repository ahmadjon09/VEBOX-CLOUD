//go:build linux

package main

import "syscall"

func diskStats(path string) (total, free, avail uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, false
	}
	bs := uint64(st.Bsize)
	return st.Blocks * bs, st.Bfree * bs, st.Bavail * bs, true
}
