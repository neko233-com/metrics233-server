//go:build darwin

package hostagent

import (
	"context"
	"errors"
	"github.com/shirou/gopsutil/v4/disk"
	"golang.org/x/sys/unix"
	"strings"
)

// A stale network mount must not stall a local-resource cycle. MNT_NOWAIT uses
// cached mount metadata; capacity reads below are limited to local filesystems.
func localPartitions(ctx context.Context, _ bool) ([]disk.PartitionStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if n < 0 || n > 4096 {
		return nil, errors.New("mount metadata limit exceeded")
	}
	stats := make([]unix.Statfs_t, n)
	n, err = unix.Getfsstat(stats, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if n > len(stats) {
		return nil, errors.New("mount metadata changed")
	}
	out := []disk.PartitionStat{}
	for _, stat := range stats[:n] {
		if stat.Flags&unix.MNT_LOCAL == 0 {
			continue
		}
		opts := []string{"rw"}
		if stat.Flags&unix.MNT_RDONLY != 0 {
			opts = []string{"ro"}
		}
		out = append(out, disk.PartitionStat{Device: strings.TrimRight(string(stat.Mntfromname[:]), "\x00"), Mountpoint: strings.TrimRight(string(stat.Mntonname[:]), "\x00"), Fstype: strings.TrimRight(string(stat.Fstypename[:]), "\x00"), Opts: opts})
	}
	return out, nil
}
