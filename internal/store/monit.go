package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type MonitReport struct {
	ID, Version, Hostname string
	Incarnation           int64
	Services              []MonitService
	Event                 *MonitEvent
}
type MonitService struct {
	Name      string
	Type      int
	Status    int64
	Monitor   int
	Collected time.Time
}
type MonitEvent struct {
	Service, Message string
	ID               int64
	State, Action    int
	Collected        time.Time
}

func (s *Store) RecordMonit(ctx context.Context, r MonitReport) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var hostID, deviceID string
	err = tx.QueryRow(ctx, `SELECT id,device_id FROM monit_hosts WHERE monit_id=$1`, r.ID).Scan(&hostID, &deviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO devices(site_id,name,address,kind,status,last_seen_at) VALUES((SELECT id FROM sites ORDER BY created_at LIMIT 1),$1,$1,'server','up',now()) RETURNING id`, r.Hostname).Scan(&deviceID)
		if err != nil {
			return "", err
		}
		err = tx.QueryRow(ctx, `INSERT INTO monit_hosts(monit_id,device_id,version,incarnation) VALUES($1,$2,$3,$4) RETURNING id`, r.ID, deviceID, r.Version, r.Incarnation).Scan(&hostID)
	} else if err == nil {
		_, err = tx.Exec(ctx, `UPDATE monit_hosts SET version=$2,incarnation=$3,last_report_at=now() WHERE id=$1`, hostID, r.Version, r.Incarnation)
	}
	if err != nil {
		return "", err
	}
	for _, v := range r.Services {
		_, err = tx.Exec(ctx, `INSERT INTO monit_services(host_id,name,type,status,monitor,collected_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(host_id,name) DO UPDATE SET type=excluded.type,status=excluded.status,monitor=excluded.monitor,collected_at=excluded.collected_at,updated_at=now()`, hostID, v.Name, v.Type, v.Status, v.Monitor, v.Collected)
		if err != nil {
			return "", err
		}
	}
	if len(r.Services) > 0 {
		names := make([]string, 0, len(r.Services))
		for _, v := range r.Services {
			names = append(names, v.Name)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM monit_services WHERE host_id=$1 AND NOT (name=ANY($2))`, hostID, names); err != nil {
			return "", err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE devices SET status=CASE WHEN EXISTS(SELECT 1 FROM monit_services WHERE host_id=$2 AND status<>0 AND monitor<>0) THEN 'down' ELSE 'up' END,last_seen_at=now(),updated_at=now() WHERE id=$1`, deviceID, hostID)
	if err != nil {
		return "", err
	}
	if r.Event != nil {
		e := r.Event
		_, err = tx.Exec(ctx, `INSERT INTO monit_events(host_id,service,event_id,state,action,message,collected_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, hostID, e.Service, e.ID, e.State, e.Action, e.Message, e.Collected)
		if err != nil {
			return "", err
		}
		key := r.ID + ":" + e.Service + ":" + fmt.Sprint(e.ID)
		if e.State != 0 {
			_, err = tx.Exec(ctx, `INSERT INTO incidents(check_id,device_id,title,source,source_key) VALUES(NULL,$1,$2,'monit',$3) ON CONFLICT DO NOTHING`, deviceID, e.Service+": "+e.Message, key)
		} else {
			_, err = tx.Exec(ctx, `UPDATE incidents SET state='resolved',resolved_at=now() WHERE source='monit' AND source_key=$1 AND state IN ('open','acknowledged')`, key)
		}
		if err != nil {
			return "", err
		}
	}
	return deviceID, tx.Commit(ctx)
}

func (s *Store) MonitServices(ctx context.Context) ([]MonitServiceStatus, error) {
	rows, err := s.Pool.Query(ctx, `SELECT d.id,d.name,coalesce(d.site_id::text,''),h.monit_id,h.version,h.last_report_at,m.name,m.type,m.status,m.monitor,m.collected_at,m.updated_at FROM monit_services m JOIN monit_hosts h ON h.id=m.host_id JOIN devices d ON d.id=h.device_id ORDER BY d.name,m.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MonitServiceStatus{}
	for rows.Next() {
		var v MonitServiceStatus
		if err = rows.Scan(&v.DeviceID, &v.DeviceName, &v.SiteID, &v.MonitID, &v.Version, &v.LastReportAt, &v.Name, &v.Type, &v.Status, &v.Monitor, &v.CollectedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
