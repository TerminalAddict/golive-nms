package store

import (
	"context"
	"errors"
	"time"
)

type NotificationChannel struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	CredentialID   string `json:"credentialId"`
	Enabled        bool   `json:"enabled"`
	SiteID         string `json:"siteId"`
	NotifyOpened   bool   `json:"notifyOpened"`
	NotifyResolved bool   `json:"notifyResolved"`
	RepeatMinutes  int    `json:"repeatMinutes"`
}

func (s *Store) NotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,kind,credential_id,enabled,coalesce(site_id::text,''),notify_opened,notify_resolved,repeat_minutes FROM notification_channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationChannel{}
	for rows.Next() {
		var c NotificationChannel
		if err = rows.Scan(&c.ID, &c.Name, &c.Kind, &c.CredentialID, &c.Enabled, &c.SiteID, &c.NotifyOpened, &c.NotifyResolved, &c.RepeatMinutes); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) CreateNotificationChannel(ctx context.Context, c NotificationChannel) (NotificationChannel, error) {
	if c.Kind != "email" && c.Kind != "slack" && c.Kind != "teams" {
		return c, errors.New("invalid notification channel kind")
	}
	c.RepeatMinutes = 1440
	var credentialKind string
	if err := s.Pool.QueryRow(ctx, `SELECT kind FROM credentials WHERE id=$1`, c.CredentialID).Scan(&credentialKind); err != nil {
		return c, errors.New("notification credential not found")
	}
	if (c.Kind == "email" && credentialKind != "smtp") || (c.Kind != "email" && credentialKind != "webhook") {
		return c, errors.New("notification credential type does not match the channel")
	}
	c.Enabled = true
	if !c.NotifyOpened && !c.NotifyResolved {
		c.NotifyOpened = true
		c.NotifyResolved = true
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO notification_channels(name,kind,credential_id,site_id,notify_opened,notify_resolved,repeat_minutes) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7) RETURNING id,enabled`, c.Name, c.Kind, c.CredentialID, c.SiteID, c.NotifyOpened, c.NotifyResolved, c.RepeatMinutes).Scan(&c.ID, &c.Enabled)
	return c, err
}
func (s *Store) DeleteNotificationChannel(ctx context.Context, id string) error {
	var credentialID string
	if err := s.Pool.QueryRow(ctx, `SELECT credential_id FROM notification_channels WHERE id=$1`, id).Scan(&credentialID); err != nil {
		return errors.New("notification channel not found")
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM notification_channels WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("notification channel not found")
	}
	if err == nil {
		_, _ = s.Pool.Exec(ctx, `DELETE FROM credentials WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM notification_channels WHERE credential_id=$1)`, credentialID)
	}
	return err
}
func (s *Store) NotificationChannel(ctx context.Context, id string) (NotificationChannel, error) {
	var c NotificationChannel
	err := s.Pool.QueryRow(ctx, `SELECT id,name,kind,credential_id,enabled,coalesce(site_id::text,''),notify_opened,notify_resolved,repeat_minutes FROM notification_channels WHERE id=$1`, id).Scan(&c.ID, &c.Name, &c.Kind, &c.CredentialID, &c.Enabled, &c.SiteID, &c.NotifyOpened, &c.NotifyResolved, &c.RepeatMinutes)
	return c, err
}

func (s *Store) RecordDelivery(ctx context.Context, channelID, incidentID, deviceID, event string, notificationEventID int64, success bool, message string) {
	_, _ = s.Pool.Exec(ctx, `INSERT INTO notification_deliveries(channel_id,incident_id,device_id,event,notification_event_id,success,error) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,NULLIF($5,0),$6,$7) ON CONFLICT DO NOTHING`, channelID, incidentID, deviceID, event, notificationEventID, success, message)
}

func (s *Store) NotificationEventDelivered(ctx context.Context, channelID string, eventID int64) bool {
	var delivered bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_deliveries WHERE channel_id=$1 AND notification_event_id=$2 AND success)`, channelID, eventID).Scan(&delivered)
	return delivered
}

type NotificationReminder struct {
	Channel                                 NotificationChannel
	IncidentID, DeviceID, DeviceName, Title string
}

func (s *Store) NotificationReminders(ctx context.Context) ([]NotificationReminder, error) {
	rows, err := s.Pool.Query(ctx, `SELECT n.id,n.name,n.kind,n.credential_id,n.enabled,coalesce(n.site_id::text,''),n.notify_opened,n.notify_resolved,n.repeat_minutes,i.id,d.id,d.name,i.title FROM notification_channels n JOIN incidents i ON i.state='open' JOIN devices d ON d.id=i.device_id WHERE n.enabled AND n.notify_opened AND n.repeat_minutes>0 AND i.opened_at<=now()-(n.repeat_minutes*interval '1 minute') AND (n.site_id IS NULL OR n.site_id=d.site_id) AND NOT EXISTS(SELECT 1 FROM notification_deliveries x WHERE x.channel_id=n.id AND x.incident_id=i.id AND x.event IN ('opened','reminder') AND x.success AND x.created_at>now()-(n.repeat_minutes*interval '1 minute'))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationReminder{}
	for rows.Next() {
		var v NotificationReminder
		if err = rows.Scan(&v.Channel.ID, &v.Channel.Name, &v.Channel.Kind, &v.Channel.CredentialID, &v.Channel.Enabled, &v.Channel.SiteID, &v.Channel.NotifyOpened, &v.Channel.NotifyResolved, &v.Channel.RepeatMinutes, &v.IncidentID, &v.DeviceID, &v.DeviceName, &v.Title); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type NotificationEvent struct {
	ID                                             int64
	IncidentID, DeviceID, DeviceName, Title, State string
	CreatedAt                                      time.Time
}

func (s *Store) ClaimNotificationEvents(ctx context.Context, limit int) ([]NotificationEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH pending AS (
 SELECT id FROM notification_events
 WHERE processed_at IS NULL AND next_attempt_at<=now() AND (processing_at IS NULL OR processing_at<now()-interval '2 minutes')
 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1
) UPDATE notification_events e SET processing_at=now(),attempts=attempts+1
FROM pending p WHERE e.id=p.id
RETURNING e.id,e.incident_id,e.event,e.created_at`, limit)
	if err != nil {
		return nil, err
	}
	events := []NotificationEvent{}
	for rows.Next() {
		var event NotificationEvent
		if err = rows.Scan(&event.ID, &event.IncidentID, &event.State, &event.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		events = append(events, event)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range events {
		err = tx.QueryRow(ctx, `SELECT i.device_id,d.name,i.title FROM incidents i JOIN devices d ON d.id=i.device_id WHERE i.id=$1`, events[i].IncidentID).Scan(&events[i].DeviceID, &events[i].DeviceName, &events[i].Title)
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) CompleteNotificationEvent(ctx context.Context, id int64, deliveryErr error) {
	if deliveryErr == nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE notification_events SET processed_at=now(),processing_at=NULL,last_error='' WHERE id=$1`, id)
		return
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE notification_events SET processing_at=NULL,next_attempt_at=now()+interval '1 minute',last_error=$2 WHERE id=$1`, id, deliveryErr.Error())
}
