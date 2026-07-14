package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type MonitControl struct {
	DeviceID, URL, CredentialID, CredentialName string
	UpdatedAt                                   time.Time
}

type MonitAction struct {
	ID, DeviceID, Service, Action, Message string
	Success                                bool
	RequestedAt                            time.Time
}

func (s *Store) MonitControl(ctx context.Context, deviceID string) (MonitControl, error) {
	var v MonitControl
	err := s.Pool.QueryRow(ctx, `SELECT mc.device_id,mc.url,mc.credential_id,c.name,mc.updated_at FROM monit_controls mc JOIN credentials c ON c.id=mc.credential_id WHERE mc.device_id=$1`, deviceID).Scan(&v.DeviceID, &v.URL, &v.CredentialID, &v.CredentialName, &v.UpdatedAt)
	return v, err
}

func (s *Store) SetMonitControl(ctx context.Context, v MonitControl) (MonitControl, error) {
	var kind string
	if err := s.Pool.QueryRow(ctx, `SELECT kind FROM credentials WHERE id=$1`, v.CredentialID).Scan(&kind); err != nil {
		return v, errors.New("Monit credential not found")
	}
	if kind != "monit" {
		return v, errors.New("credential must be a Monit credential")
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO monit_controls(device_id,url,credential_id) VALUES($1,$2,$3) ON CONFLICT(device_id) DO UPDATE SET url=excluded.url,credential_id=excluded.credential_id,updated_at=now() RETURNING updated_at`, v.DeviceID, v.URL, v.CredentialID).Scan(&v.UpdatedAt)
	return v, err
}

func (s *Store) MonitServiceExists(ctx context.Context, deviceID, service string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monit_services ms JOIN monit_hosts mh ON mh.id=ms.host_id WHERE mh.device_id=$1 AND ms.name=$2)`, deviceID, service).Scan(&exists)
	return exists, err
}

func (s *Store) RecordMonitAction(ctx context.Context, v MonitAction, userID string) (MonitAction, error) {
	err := s.Pool.QueryRow(ctx, `INSERT INTO monit_actions(device_id,service,action,requested_by,success,message) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6) RETURNING id,requested_at`, v.DeviceID, v.Service, v.Action, userID, v.Success, v.Message).Scan(&v.ID, &v.RequestedAt)
	return v, err
}

func (s *Store) MonitActions(ctx context.Context, deviceID string) ([]MonitAction, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,device_id,service,action,success,message,requested_at FROM monit_actions WHERE device_id=$1 ORDER BY requested_at DESC LIMIT 20`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MonitAction{}
	for rows.Next() {
		var v MonitAction
		if err = rows.Scan(&v.ID, &v.DeviceID, &v.Service, &v.Action, &v.Success, &v.Message, &v.RequestedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
