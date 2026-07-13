package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type ConfigProfile struct {
	ID              string     `json:"id"`
	DeviceID        string     `json:"deviceId"`
	DeviceName      string     `json:"deviceName"`
	Address         string     `json:"address"`
	SiteID          string     `json:"siteId"`
	CredentialID    string     `json:"credentialId"`
	Command         string     `json:"command"`
	IntervalSeconds int        `json:"intervalSeconds"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"lastRunAt"`
	LastError       string     `json:"lastError"`
}
type ConfigSnapshot struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"deviceId"`
	ContentHash string    `json:"contentHash"`
	CapturedAt  time.Time `json:"capturedAt"`
}

func (s *Store) ConfigProfiles(ctx context.Context) ([]ConfigProfile, error) {
	rows, err := s.Pool.Query(ctx, `SELECT p.id,p.device_id,d.name,d.address,coalesce(d.site_id::text,''),p.credential_id,p.command,p.interval_seconds,p.enabled,p.last_run_at,p.last_error FROM config_backup_profiles p JOIN devices d ON d.id=p.device_id ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigProfile{}
	for rows.Next() {
		var v ConfigProfile
		if err = rows.Scan(&v.ID, &v.DeviceID, &v.DeviceName, &v.Address, &v.SiteID, &v.CredentialID, &v.Command, &v.IntervalSeconds, &v.Enabled, &v.LastRunAt, &v.LastError); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateConfigProfile(ctx context.Context, v ConfigProfile) (ConfigProfile, error) {
	if v.Command == "" {
		return v, errors.New("backup command is required")
	}
	if v.IntervalSeconds == 0 {
		v.IntervalSeconds = 86400
	}
	v.Enabled = true
	err := s.Pool.QueryRow(ctx, `INSERT INTO config_backup_profiles(device_id,credential_id,command,interval_seconds) VALUES($1,$2,$3,$4) ON CONFLICT(device_id) DO UPDATE SET credential_id=excluded.credential_id,command=excluded.command,interval_seconds=excluded.interval_seconds,enabled=true,next_run_at=now() RETURNING id,enabled`, v.DeviceID, v.CredentialID, v.Command, v.IntervalSeconds).Scan(&v.ID, &v.Enabled)
	return v, err
}
func (s *Store) DeleteConfigProfile(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM config_backup_profiles WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("profile not found")
	}
	return err
}
func (s *Store) ClaimConfigBackup(ctx context.Context) (ConfigProfile, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ConfigProfile{}, err
	}
	defer tx.Rollback(ctx)
	var v ConfigProfile
	err = tx.QueryRow(ctx, `SELECT p.id,p.device_id,d.name,d.address,coalesce(d.site_id::text,''),p.credential_id,p.command,p.interval_seconds,p.enabled,p.last_run_at,p.last_error FROM config_backup_profiles p JOIN devices d ON d.id=p.device_id WHERE p.enabled AND p.next_run_at<=now() ORDER BY p.next_run_at FOR UPDATE OF p SKIP LOCKED LIMIT 1`).Scan(&v.ID, &v.DeviceID, &v.DeviceName, &v.Address, &v.SiteID, &v.CredentialID, &v.Command, &v.IntervalSeconds, &v.Enabled, &v.LastRunAt, &v.LastError)
	if err != nil {
		return v, err
	}
	_, err = tx.Exec(ctx, `UPDATE config_backup_profiles SET next_run_at=now()+(interval_seconds*interval '1 second') WHERE id=$1`, v.ID)
	if err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Store) ConfigBackupFailed(ctx context.Context, id, message string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE config_backup_profiles SET last_run_at=now(),last_error=$2 WHERE id=$1`, id, message)
}
func (s *Store) SaveConfigSnapshot(ctx context.Context, v ConfigProfile, content []byte) (ConfigSnapshot, bool, error) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	encrypted, err := s.Vault.Seal(content)
	if err != nil {
		return ConfigSnapshot{}, false, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ConfigSnapshot{}, false, err
	}
	defer tx.Rollback(ctx)
	var previous string
	_ = tx.QueryRow(ctx, `SELECT content_hash FROM config_snapshots WHERE device_id=$1 ORDER BY captured_at DESC LIMIT 1`, v.DeviceID).Scan(&previous)
	var snap ConfigSnapshot
	err = tx.QueryRow(ctx, `INSERT INTO config_snapshots(device_id,content_hash,encrypted_content) VALUES($1,$2,$3) ON CONFLICT(device_id,content_hash) DO UPDATE SET captured_at=excluded.captured_at RETURNING id,device_id,content_hash,captured_at`, v.DeviceID, hash, encrypted).Scan(&snap.ID, &snap.DeviceID, &snap.ContentHash, &snap.CapturedAt)
	if err != nil {
		return snap, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE config_backup_profiles SET last_run_at=now(),last_error='' WHERE id=$1`, v.ID)
	if err != nil {
		return snap, false, err
	}
	changed := previous != "" && previous != hash
	if changed {
		_, err = tx.Exec(ctx, `INSERT INTO incidents(check_id,device_id,title,severity,source,source_key) VALUES(NULL,$1,$2,'warning','config',$3) ON CONFLICT DO NOTHING`, v.DeviceID, "Configuration changed on "+v.DeviceName, v.DeviceID)
	}
	if err != nil {
		return snap, false, err
	}
	return snap, changed, tx.Commit(ctx)
}
func (s *Store) ConfigSnapshots(ctx context.Context, deviceID string) ([]ConfigSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,device_id,content_hash,captured_at FROM config_snapshots WHERE device_id=$1 ORDER BY captured_at DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigSnapshot{}
	for rows.Next() {
		var v ConfigSnapshot
		if err = rows.Scan(&v.ID, &v.DeviceID, &v.ContentHash, &v.CapturedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) ConfigContent(ctx context.Context, id string) ([]byte, error) {
	var encrypted []byte
	err := s.Pool.QueryRow(ctx, `SELECT encrypted_content FROM config_snapshots WHERE id=$1`, id).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("snapshot not found")
	}
	if err != nil {
		return nil, err
	}
	return s.Vault.Open(encrypted)
}
func (s *Store) ConfigSnapshotDevice(ctx context.Context, id string) (string, error) {
	var device string
	err := s.Pool.QueryRow(ctx, `SELECT device_id FROM config_snapshots WHERE id=$1`, id).Scan(&device)
	return device, err
}
func (s *Store) TriggerConfigBackup(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE config_backup_profiles SET next_run_at=now() WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("profile not found")
	}
	return err
}
