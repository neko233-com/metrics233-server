//go:build linux || darwin

package store

import (
	"errors"
	"os"
	"syscall"
)

func syncStorageDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	return errors.Join(err, f.Close())
}

func allocatedFileBytes(info os.FileInfo) int64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Blocks * 512
	}
	return info.Size()
}
