package store

import (
	"context"
	"encoding/json"
	"time"
)

type DeviceEvent struct {
	ID         int64          `json:"id"`
	DeviceID   string         `json:"deviceId"`
	Protocol   string         `json:"protocol"`
	Source     string         `json:"source"`
	Facility   *int           `json:"facility"`
	Severity   *int           `json:"severity"`
	Message    string         `json:"message"`
	Fields     map[string]any `json:"fields"`
	ReceivedAt time.Time      `json:"receivedAt"`
	SiteID     string         `json:"siteId"`
}

func (s *Store) RecordDeviceEvent(ctx context.Context, e DeviceEvent) error {
	fields, err := json.Marshal(e.Fields)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO device_events(device_id,protocol,source,facility,severity,message,fields) VALUES((SELECT id FROM devices WHERE address=$1 ORDER BY created_at LIMIT 1),$2,$1,$3,$4,$5,$6)`, e.Source, e.Protocol, e.Facility, e.Severity, e.Message, fields)
	return err
}
func (s *Store) DeviceEvents(ctx context.Context, protocol, query string, limit int) ([]DeviceEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.Pool.Query(ctx, `SELECT e.id,coalesce(e.device_id::text,''),e.protocol,e.source,e.facility,e.severity,e.message,e.fields,e.received_at,coalesce(d.site_id::text,'') FROM device_events e LEFT JOIN devices d ON d.id=e.device_id WHERE ($1='' OR e.protocol=$1) AND ($2='' OR e.message ILIKE '%'||$2||'%' OR e.source ILIKE '%'||$2||'%') ORDER BY e.received_at DESC LIMIT $3`, protocol, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceEvent{}
	for rows.Next() {
		var e DeviceEvent
		var raw []byte
		if err = rows.Scan(&e.ID, &e.DeviceID, &e.Protocol, &e.Source, &e.Facility, &e.Severity, &e.Message, &raw, &e.ReceivedAt, &e.SiteID); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &e.Fields)
		out = append(out, e)
	}
	return out, rows.Err()
}
