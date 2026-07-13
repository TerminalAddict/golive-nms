package store

import (
	"context"
	"errors"
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
	if c.RepeatMinutes < 0 || c.RepeatMinutes > 1440 {
		return c, errors.New("repeat interval must be between 0 and 1440 minutes")
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
	tag, err := s.Pool.Exec(ctx, `DELETE FROM notification_channels WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("notification channel not found")
	}
	return err
}
func (s *Store) RecordDelivery(ctx context.Context, channelID, incidentID, deviceID, event string, success bool, message string) {
	_, _ = s.Pool.Exec(ctx, `INSERT INTO notification_deliveries(channel_id,incident_id,device_id,event,success,error) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6)`, channelID, incidentID, deviceID, event, success, message)
}

type NotificationReminder struct {
	Channel                                 NotificationChannel
	IncidentID, DeviceID, DeviceName, Title string
}

func (s *Store) NotificationReminders(ctx context.Context) ([]NotificationReminder, error) {
	rows, err := s.Pool.Query(ctx, `SELECT n.id,n.name,n.kind,n.credential_id,n.enabled,coalesce(n.site_id::text,''),n.notify_opened,n.notify_resolved,n.repeat_minutes,i.id,d.id,d.name,i.title FROM notification_channels n JOIN incidents i ON i.state IN ('open','acknowledged') JOIN devices d ON d.id=i.device_id WHERE n.enabled AND n.notify_opened AND n.repeat_minutes>0 AND (n.site_id IS NULL OR n.site_id=d.site_id) AND NOT EXISTS(SELECT 1 FROM notification_deliveries x WHERE x.channel_id=n.id AND x.device_id=d.id AND x.created_at>now()-(n.repeat_minutes*interval '1 minute'))`)
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
