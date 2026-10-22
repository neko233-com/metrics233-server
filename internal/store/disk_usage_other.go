//go:build !linux && !darwin

package store

import "os"

func syncStorageDirectory(string) error { return nil }

func allocatedFileBytes(info os.FileInfo) int64 { return info.Size() }
