//go:build !linux

package main

func diskStats(path string) (total, free, avail uint64, ok bool) {
	return 0, 0, 0, false
}
