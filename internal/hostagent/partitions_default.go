//go:build !darwin

package hostagent

import (
	"context"
	"github.com/shirou/gopsutil/v4/disk"
)

func localPartitions(ctx context.Context, all bool) ([]disk.PartitionStat, error) {
	return disk.PartitionsWithContext(ctx, all)
}
