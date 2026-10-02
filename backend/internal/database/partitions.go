package database

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"gorm.io/gorm"
)

// PartitionInfo describes one telemetry partition for the storage console.
type PartitionInfo struct {
	Name      string    `json:"name"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Rows      int64     `json:"rows"`
	SizeBytes int64     `json:"sizeBytes"`
}

var partitionNameRe = regexp.MustCompile(`^telemetry_data_(\d{6})$`)

// ListPartitions returns telemetry partitions with estimated row counts and
// on-disk sizes, oldest first.
func ListPartitions(ctx context.Context, db *gorm.DB) ([]PartitionInfo, error) {
	rows, err := db.WithContext(ctx).Raw(`
		SELECT c.relname AS name,
		       GREATEST(c.reltuples, 0)::bigint AS rows,
		       pg_total_relation_size(c.oid) AS size_bytes
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'telemetry_data'
		ORDER BY c.relname`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PartitionInfo
	for rows.Next() {
		var info PartitionInfo
		if err := rows.Scan(&info.Name, &info.Rows, &info.SizeBytes); err != nil {
			return nil, err
		}
		if m := partitionNameRe.FindStringSubmatch(info.Name); m != nil {
			if from, err := time.Parse("200601", m[1]); err == nil {
				info.From = from
				info.To = from.AddDate(0, 1, 0)
			}
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// DropPartition removes a telemetry partition by name. The name is validated
// against the expected pattern to prevent SQL injection.
func DropPartition(ctx context.Context, db *gorm.DB, name string) error {
	if !partitionNameRe.MatchString(name) {
		return fmt.Errorf("invalid partition name %q", name)
	}
	return db.WithContext(ctx).Exec("DROP TABLE IF EXISTS " + name).Error
}
