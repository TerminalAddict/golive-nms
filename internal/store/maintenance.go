package store

import (
	"context"
	"errors"
	"time"
)

type MaintenanceWindow struct {
	ID, SiteID, DeviceID, Name string
	StartsAt, EndsAt           time.Time
}

func (s *Store) MaintenanceWindows(ctx context.Context) ([]MaintenanceWindow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,coalesce(site_id::text,''),coalesce(device_id::text,''),name,starts_at,ends_at FROM maintenance_windows WHERE ends_at>now()-interval '7 days' ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MaintenanceWindow{}
	for rows.Next() {
		var v MaintenanceWindow
		if err = rows.Scan(&v.ID, &v.SiteID, &v.DeviceID, &v.Name, &v.StartsAt, &v.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateMaintenanceWindow(ctx context.Context, v MaintenanceWindow, userID string) (MaintenanceWindow, error) {
	if v.Name == "" || (v.SiteID == "" && v.DeviceID == "") || !v.EndsAt.After(v.StartsAt) || v.EndsAt.Sub(v.StartsAt) > 366*24*time.Hour {
		return v, errors.New("name, target, and a valid window of at most one year are required")
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO maintenance_windows(site_id,device_id,name,starts_at,ends_at,created_by) VALUES(NULLIF($1,'')::uuid,NULLIF($2,'')::uuid,$3,$4,$5,$6) RETURNING id`, v.SiteID, v.DeviceID, v.Name, v.StartsAt, v.EndsAt, userID).Scan(&v.ID)
	return v, err
}
func (s *Store) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM maintenance_windows WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("maintenance window not found")
	}
	return err
}
func (s *Store) DeviceInMaintenance(ctx context.Context, deviceID string) (bool, error) {
	var v bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM maintenance_windows m JOIN devices d ON d.id=$1 WHERE now() BETWEEN m.starts_at AND m.ends_at AND (m.device_id=d.id OR m.site_id=d.site_id))`, deviceID).Scan(&v)
	return v, err
}
