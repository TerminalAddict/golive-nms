package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/vault"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool  *pgxpool.Pool
	Vault *vault.Cipher
}

type Device struct {
	ID, SiteID, SiteName, ParentID, Name, Address, Kind, Status string
	Tags                                                        []string
	LastSeenAt                                                  *time.Time
}
type MonitServiceStatus struct {
	DeviceID, DeviceName, SiteID, MonitID, Version, Name string
	Type, Monitor                                        int
	Status                                               int64
	CollectedAt, UpdatedAt                               *time.Time
	LastReportAt                                         time.Time
}
type Check struct {
	ID, DeviceID, DeviceName, Name, Type, Target, Status, LastError string
	SiteID                                                          string
	CredentialID                                                    string
	Config                                                          json.RawMessage
	IntervalSeconds, TimeoutSeconds                                 int
	Enabled                                                         bool
	LastRunAt                                                       *time.Time
}
type Incident struct {
	ID, CheckID, DeviceID, DeviceName, Title, Severity, State string
	SiteID                                                    string
	AssignedTo, AssignedName, Notes                           string
	OpenedAt                                                  time.Time
	AcknowledgedAt, ResolvedAt                                *time.Time
}
type Summary struct{ Total, Up, Down, Degraded, Unknown, OpenIncidents int }
type AgentReport struct {
	AgentID  string         `json:"agentId"`
	Hostname string         `json:"hostname"`
	Address  string         `json:"address"`
	Version  string         `json:"version"`
	Metrics  map[string]any `json:"metrics"`
}
type AgentInventory struct {
	DeviceID, DeviceName, SiteID, AgentID, Version string
	Metrics                                        map[string]any
	ReportedAt                                     time.Time
}

func Open(ctx context.Context, url string, migrations embed.FS, encryptionKey string) (*Store, error) {
	cipher, err := vault.New(encryptionKey)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		pool.Close()
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	if _, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		pool.Close()
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied bool
		if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, entry.Name()).Scan(&applied); e != nil {
			pool.Close()
			return nil, e
		}
		if applied {
			continue
		}
		sql, e := migrations.ReadFile("migrations/" + entry.Name())
		if e != nil {
			pool.Close()
			return nil, e
		}
		if _, e = pool.Exec(ctx, string(sql)); e != nil {
			pool.Close()
			return nil, fmt.Errorf("migration %s: %w", entry.Name(), e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, entry.Name()); e != nil {
			pool.Close()
			return nil, e
		}
	}
	return &Store{Pool: pool, Vault: cipher}, nil
}
func (s *Store) Close() { s.Pool.Close() }

func (s *Store) Summary(ctx context.Context) (Summary, error) {
	var x Summary
	err := s.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='up'),count(*) FILTER(WHERE status='down'),count(*) FILTER(WHERE status='degraded'),count(*) FILTER(WHERE status IN ('unknown','dependency')),(SELECT count(*) FROM incidents WHERE state IN ('open','acknowledged')) FROM devices`).Scan(&x.Total, &x.Up, &x.Down, &x.Degraded, &x.Unknown, &x.OpenIncidents)
	return x, err
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.Pool.Query(ctx, `SELECT d.id,coalesce(d.site_id::text,''),coalesce(s.name,''),coalesce(d.parent_id::text,''),d.name,d.address,d.kind,d.status,d.tags,d.last_seen_at FROM devices d LEFT JOIN sites s ON s.id=d.site_id ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err = rows.Scan(&d.ID, &d.SiteID, &d.SiteName, &d.ParentID, &d.Name, &d.Address, &d.Kind, &d.Status, &d.Tags, &d.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateDevice(ctx context.Context, d Device) (Device, error) {
	if d.Kind == "" {
		d.Kind = "server"
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO devices(site_id,parent_id,name,address,kind,tags) VALUES(coalesce(NULLIF($1,'')::uuid,(SELECT id FROM sites ORDER BY created_at LIMIT 1)),NULLIF($2,'')::uuid,$3,$4,$5,$6) RETURNING id,status,site_id::text`, d.SiteID, d.ParentID, d.Name, d.Address, d.Kind, d.Tags).Scan(&d.ID, &d.Status, &d.SiteID)
	return d, err
}

func (s *Store) UpdateDevice(ctx context.Context, d Device) (Device, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	var crossSiteChildren bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE parent_id=$1 AND site_id IS DISTINCT FROM $2::uuid)`, d.ID, d.SiteID).Scan(&crossSiteChildren); err != nil {
		return d, err
	}
	if crossSiteChildren {
		return d, errors.New("move or detach child devices before changing this device's site")
	}
	if d.ParentID != "" {
		if d.ParentID == d.ID {
			return d, errors.New("a device cannot be its own parent")
		}
		var parentSite string
		if err = tx.QueryRow(ctx, `SELECT coalesce(site_id::text,'') FROM devices WHERE id=$1`, d.ParentID).Scan(&parentSite); err != nil {
			return d, errors.New("parent device not found")
		}
		if parentSite != d.SiteID {
			return d, errors.New("parent device must belong to the same site")
		}
		var cycle bool
		err = tx.QueryRow(ctx, `WITH RECURSIVE ancestors AS (SELECT id,parent_id FROM devices WHERE id=$1 UNION ALL SELECT d.id,d.parent_id FROM devices d JOIN ancestors a ON d.id=a.parent_id) SELECT EXISTS(SELECT 1 FROM ancestors WHERE id=$2)`, d.ParentID, d.ID).Scan(&cycle)
		if err != nil {
			return d, err
		}
		if cycle {
			return d, errors.New("parent relationship would create a cycle")
		}
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	err = tx.QueryRow(ctx, `UPDATE devices SET site_id=$2,parent_id=NULLIF($3,'')::uuid,name=$4,address=$5,kind=$6,tags=$7,updated_at=now() WHERE id=$1 RETURNING status,last_seen_at`, d.ID, d.SiteID, d.ParentID, d.Name, d.Address, d.Kind, d.Tags).Scan(&d.Status, &d.LastSeenAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return d, errors.New("device not found")
		}
		return d, err
	}
	if err = tx.QueryRow(ctx, `SELECT coalesce(name,'') FROM sites WHERE id=$1`, d.SiteID).Scan(&d.SiteName); err != nil {
		return d, errors.New("site not found")
	}
	return d, tx.Commit(ctx)
}

func (s *Store) DeleteDevice(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM devices WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return fmt.Errorf("device not found")
	}
	return err
}

func (s *Store) Checks(ctx context.Context) ([]Check, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.device_id,d.name,c.name,c.type,c.target,c.interval_seconds,c.timeout_seconds,c.enabled,c.status,c.last_error,c.last_run_at,coalesce(c.credential_id::text,''),c.config,coalesce(d.site_id::text,'') FROM checks c JOIN devices d ON d.id=c.device_id ORDER BY d.name,c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		var c Check
		if err = rows.Scan(&c.ID, &c.DeviceID, &c.DeviceName, &c.Name, &c.Type, &c.Target, &c.IntervalSeconds, &c.TimeoutSeconds, &c.Enabled, &c.Status, &c.LastError, &c.LastRunAt, &c.CredentialID, &c.Config, &c.SiteID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateCheck(ctx context.Context, c Check) (Check, error) {
	if c.IntervalSeconds == 0 {
		c.IntervalSeconds = 30
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 5
	}
	c.Enabled = true
	if len(c.Config) == 0 {
		c.Config = json.RawMessage(`{}`)
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO checks(device_id,name,type,target,interval_seconds,timeout_seconds,credential_id,config)VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8)RETURNING id,status`, c.DeviceID, c.Name, c.Type, c.Target, c.IntervalSeconds, c.TimeoutSeconds, c.CredentialID, c.Config).Scan(&c.ID, &c.Status)
	return c, err
}

func (s *Store) Incidents(ctx context.Context) ([]Incident, error) {
	rows, err := s.Pool.Query(ctx, `SELECT i.id,coalesce(i.check_id::text,''),i.device_id,d.name,i.title,i.severity,i.state,i.opened_at,i.acknowledged_at,i.resolved_at,coalesce(d.site_id::text,''),coalesce(i.assigned_to::text,''),coalesce(u.display_name,''),i.notes FROM incidents i JOIN devices d ON d.id=i.device_id LEFT JOIN users u ON u.id=i.assigned_to ORDER BY i.opened_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		var i Incident
		if err = rows.Scan(&i.ID, &i.CheckID, &i.DeviceID, &i.DeviceName, &i.Title, &i.Severity, &i.State, &i.OpenedAt, &i.AcknowledgedAt, &i.ResolvedAt, &i.SiteID, &i.AssignedTo, &i.AssignedName, &i.Notes); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) AssignIncident(ctx context.Context, id, userID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE incidents SET assigned_to=NULLIF($2,'')::uuid WHERE id=$1 AND state IN ('open','acknowledged')`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("active incident not found")
	}
	return err
}
func (s *Store) NoteIncident(ctx context.Context, id, note string) error {
	note = strings.TrimSpace(note)
	if note == "" || len(note) > 4000 {
		return errors.New("note must contain 1 to 4000 characters")
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE incidents SET notes=CASE WHEN notes='' THEN $2 ELSE notes||E'\n'||$2 END WHERE id=$1`, id, note)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("incident not found")
	}
	return err
}

func (s *Store) Acknowledge(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE incidents SET state='acknowledged',acknowledged_at=now() WHERE id=$1 AND state='open'`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return fmt.Errorf("open incident not found")
	}
	return err
}

type DueCheck struct {
	CheckID, DeviceID, DeviceName, Type, Target string
	Status                                      string
	ParentDown                                  bool
	Maintenance                                 bool
	CredentialID                                string
	Config                                      json.RawMessage
	TimeoutSeconds                              int
}

func (s *Store) ClaimDue(ctx context.Context) (*DueCheck, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var c DueCheck
	err = tx.QueryRow(ctx, `SELECT c.id,c.device_id,d.name,c.type,c.target,c.timeout_seconds,coalesce(c.credential_id::text,''),c.config,c.status,coalesce((SELECT p.status IN ('down','dependency') FROM devices p WHERE p.id=d.parent_id),false),EXISTS(SELECT 1 FROM maintenance_windows m WHERE now() BETWEEN m.starts_at AND m.ends_at AND (m.device_id=d.id OR m.site_id=d.site_id)) FROM checks c JOIN devices d ON d.id=c.device_id LEFT JOIN sites s ON s.id=d.site_id LEFT JOIN enrolled_identities ei ON ei.id=s.collector_identity_id WHERE c.enabled AND c.next_run_at<=now() AND (ei.id IS NULL OR ei.revoked_at IS NOT NULL OR ei.last_seen_at IS NULL OR ei.last_seen_at<now()-interval '2 minutes') ORDER BY c.next_run_at FOR UPDATE OF c SKIP LOCKED LIMIT 1`).Scan(&c.CheckID, &c.DeviceID, &c.DeviceName, &c.Type, &c.Target, &c.TimeoutSeconds, &c.CredentialID, &c.Config, &c.Status, &c.ParentDown, &c.Maintenance)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE checks SET next_run_at=now()+(interval_seconds*interval '1 second') WHERE id=$1`, c.CheckID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) RecordResult(ctx context.Context, c DueCheck, up bool, latencyMS float64, message string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status := "down"
	if up {
		status = "up"
	}
	_, err = tx.Exec(ctx, `UPDATE checks SET status=$2,last_error=$3,last_run_at=now() WHERE id=$1`, c.CheckID, status, message)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO check_samples(check_id,up,latency_ms,message) VALUES($1,$2,$3,$4)`, c.CheckID, up, latencyMS, message); err != nil {
		return err
	}
	if up {
		_, err = tx.Exec(ctx, `UPDATE incidents SET state='resolved',resolved_at=now() WHERE check_id=$1 AND state IN ('open','acknowledged')`, c.CheckID)
	} else if !c.Maintenance {
		_, err = tx.Exec(ctx, `INSERT INTO incidents(check_id,device_id,title) SELECT $1,$2,$3 WHERE NOT EXISTS(SELECT 1 FROM devices child JOIN devices parent ON parent.id=child.parent_id WHERE child.id=$2 AND parent.status IN ('down','dependency')) ON CONFLICT DO NOTHING`, c.CheckID, c.DeviceID, c.DeviceName+" is unavailable")
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE devices d SET status=CASE WHEN EXISTS(SELECT 1 FROM devices p WHERE p.id=d.parent_id AND p.status IN ('down','dependency')) THEN 'dependency' WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='down') THEN 'down' WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='up') THEN 'up' ELSE 'unknown' END,last_seen_at=CASE WHEN $2 THEN now() ELSE last_seen_at END,updated_at=now() WHERE id=$1`, c.DeviceID, up)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `WITH RECURSIVE descendants AS (SELECT id,parent_id FROM devices WHERE parent_id=$1 UNION ALL SELECT d.id,d.parent_id FROM devices d JOIN descendants x ON d.parent_id=x.id) UPDATE devices d SET status=CASE WHEN EXISTS(SELECT 1 FROM devices p WHERE p.id=d.parent_id AND p.status IN ('down','dependency')) THEN 'dependency' WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='down') THEN 'down' WHEN EXISTS(SELECT 1 FROM checks WHERE device_id=d.id AND status='up') THEN 'up' ELSE 'unknown' END,updated_at=now() WHERE d.id IN(SELECT id FROM descendants)`, c.DeviceID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE incidents i SET state='resolved',resolved_at=now() FROM devices d JOIN devices p ON p.id=d.parent_id WHERE i.device_id=d.id AND i.state IN ('open','acknowledged') AND p.status IN ('down','dependency')`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type CheckSample struct {
	ObservedAt time.Time `json:"observedAt"`
	Up         bool      `json:"up"`
	LatencyMS  float64   `json:"latencyMs"`
	Message    string    `json:"message"`
}

func (s *Store) CheckHistory(ctx context.Context, id string, since time.Time) ([]CheckSample, error) {
	rows, err := s.Pool.Query(ctx, `SELECT observed_at,up,latency_ms,message FROM check_samples WHERE check_id=$1 AND observed_at>=$2 ORDER BY observed_at`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CheckSample{}
	for rows.Next() {
		var x CheckSample
		if err = rows.Scan(&x.ObservedAt, &x.Up, &x.LatencyMS, &x.Message); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) RecordAgentReport(ctx context.Context, r AgentReport, serial string) (string, error) {
	metrics, err := json.Marshal(r.Metrics)
	if err != nil {
		return "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT device_id FROM agent_reports WHERE agent_id=$1`, r.AgentID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO devices(site_id,name,address,kind,status,last_seen_at) VALUES((SELECT id FROM sites ORDER BY created_at LIMIT 1),$1,$2,'server','up',now()) RETURNING id`, r.Hostname, r.Address).Scan(&id)
	}
	if err != nil {
		return "", err
	}
	var identityID string
	if serial != "" {
		_ = tx.QueryRow(ctx, `SELECT id FROM enrolled_identities WHERE serial=$1 AND revoked_at IS NULL`, serial).Scan(&identityID)
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_reports(device_id,agent_id,version,metrics,reported_at,identity_id) VALUES($1,$2,$3,$4,now(),NULLIF($5,'')::uuid) ON CONFLICT(agent_id) DO UPDATE SET version=excluded.version,metrics=excluded.metrics,reported_at=now(),identity_id=coalesce(excluded.identity_id,agent_reports.identity_id)`, id, r.AgentID, r.Version, metrics, identityID)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `UPDATE devices SET status='up',last_seen_at=now(),address=$2,updated_at=now() WHERE id=$1`, id, r.Address)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) AgentInventory(ctx context.Context) ([]AgentInventory, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.device_id,d.name,coalesce(d.site_id::text,''),a.agent_id,a.version,a.metrics,a.reported_at FROM agent_reports a JOIN devices d ON d.id=a.device_id ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentInventory{}
	for rows.Next() {
		var v AgentInventory
		var raw []byte
		if err = rows.Scan(&v.DeviceID, &v.DeviceName, &v.SiteID, &v.AgentID, &v.Version, &raw, &v.ReportedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v.Metrics); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RunRetention bounds high-volume operational tables and also clears expired
// authentication/enrollment material. VictoriaMetrics and VictoriaLogs apply
// their own retention settings in Compose.
func (s *Store) RunRetention(ctx context.Context, days int) {
	if days < 7 {
		days = 395
	}
	run := func() {
		_, _ = s.Pool.Exec(ctx, `UPDATE remediation_jobs SET state='expired',finished_at=now(),error='agent did not return a result before the execution lease expired' WHERE state='running' AND started_at<now()-interval '10 minutes'`)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM check_samples WHERE observed_at < now()-make_interval(days=>$1)`, days)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM device_events WHERE received_at < now()-make_interval(days=>$1)`, days)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM notification_deliveries WHERE created_at < now()-make_interval(days=>$1)`, days)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM audit_log WHERE created_at < now()-make_interval(days=>$1)`, days)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM remediation_jobs WHERE queued_at < now()-make_interval(days=>$1) AND state NOT IN ('queued','running')`, days)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
		_, _ = s.Pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE expires_at < now() OR used_at IS NOT NULL`)
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
