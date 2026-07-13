package store

import (
	"context"
	"encoding/json"
	"errors"
)

type CollectorAssignment struct {
	ID              string            `json:"id"`
	DeviceID        string            `json:"deviceId"`
	DeviceName      string            `json:"deviceName"`
	Type            string            `json:"type"`
	Target          string            `json:"target"`
	IntervalSeconds int               `json:"intervalSeconds"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	Config          json.RawMessage   `json:"config"`
	Credential      map[string]string `json:"credential,omitempty"`
}

func (s *Store) CollectorAssignments(ctx context.Context, siteID string) ([]CollectorAssignment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.device_id,d.name,c.type,c.target,c.interval_seconds,c.timeout_seconds,c.config,coalesce(c.credential_id::text,'') FROM checks c JOIN devices d ON d.id=c.device_id WHERE c.enabled AND d.site_id=$1 ORDER BY c.id`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectorAssignment{}
	for rows.Next() {
		var v CollectorAssignment
		var credID string
		if err = rows.Scan(&v.ID, &v.DeviceID, &v.DeviceName, &v.Type, &v.Target, &v.IntervalSeconds, &v.TimeoutSeconds, &v.Config, &credID); err != nil {
			return nil, err
		}
		if credID != "" {
			cred, e := s.CredentialSecret(ctx, credID)
			if e != nil {
				return nil, e
			}
			v.Credential = cred.Secret
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CollectorCheck(ctx context.Context, checkID, siteID string) (DueCheck, error) {
	var c DueCheck
	err := s.Pool.QueryRow(ctx, `SELECT c.id,c.device_id,d.name,c.type,c.target,c.timeout_seconds,coalesce(c.credential_id::text,''),c.config,c.status,coalesce((SELECT p.status IN ('down','dependency') FROM devices p WHERE p.id=d.parent_id),false),EXISTS(SELECT 1 FROM maintenance_windows m WHERE now() BETWEEN m.starts_at AND m.ends_at AND (m.device_id=d.id OR m.site_id=d.site_id)) FROM checks c JOIN devices d ON d.id=c.device_id WHERE c.id=$1 AND d.site_id=$2`, checkID, siteID).Scan(&c.CheckID, &c.DeviceID, &c.DeviceName, &c.Type, &c.Target, &c.TimeoutSeconds, &c.CredentialID, &c.Config, &c.Status, &c.ParentDown, &c.Maintenance)
	if err != nil {
		return c, err
	}
	return c, nil
}
func (s *Store) RequireCollector(ctx context.Context, serial string) (Identity, error) {
	v, err := s.IdentityBySerial(ctx, serial)
	if err != nil {
		return v, err
	}
	if v.Kind != "collector" || v.SiteID == "" {
		return v, errors.New("collector identity with site assignment required")
	}
	s.TouchIdentity(ctx, serial)
	return v, nil
}
