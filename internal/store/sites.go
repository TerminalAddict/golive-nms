package store

import (
	"context"
	"errors"
)

type Site struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (s *Store) Sites(ctx context.Context) ([]Site, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,latitude,longitude FROM sites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Site{}
	for rows.Next() {
		var v Site
		if err = rows.Scan(&v.ID, &v.Name, &v.Latitude, &v.Longitude); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateSite(ctx context.Context, v Site) (Site, error) {
	if v.Name == "" {
		return v, errors.New("site name is required")
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO sites(name,latitude,longitude) VALUES($1,$2,$3) RETURNING id`, v.Name, v.Latitude, v.Longitude).Scan(&v.ID)
	return v, err
}
func (s *Store) DeleteSite(ctx context.Context, id string) error {
	var devices int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM devices WHERE site_id=$1`, id).Scan(&devices); err != nil {
		return err
	}
	if devices > 0 {
		return errors.New("move or delete devices before deleting this site")
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sites WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("site not found")
	}
	return err
}
func (s *Store) UserSiteIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT site_id FROM user_site_grants WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (s *Store) SetUserSites(ctx context.Context, userID string, ids []string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM user_site_grants WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, `INSERT INTO user_site_grants(user_id,site_id) VALUES($1,$2)`, userID, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) DeviceSite(ctx context.Context, deviceID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT coalesce(site_id::text,'') FROM devices WHERE id=$1`, deviceID).Scan(&id)
	return id, err
}

func (s *Store) DeviceBelongsToSite(ctx context.Context, deviceID, siteID string) bool {
	if deviceID == "" || siteID == "" {
		return false
	}
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1 AND site_id=$2)`, deviceID, siteID).Scan(&ok)
	return ok
}
func (s *Store) CheckSite(ctx context.Context, checkID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT coalesce(d.site_id::text,'') FROM checks c JOIN devices d ON d.id=c.device_id WHERE c.id=$1`, checkID).Scan(&id)
	return id, err
}
