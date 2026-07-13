package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type ActionTemplate struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Executable     string   `json:"executable"`
	Arguments      []string `json:"arguments"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	AutoCheckType  string   `json:"autoCheckType"`
	Enabled        bool     `json:"enabled"`
}
type RemediationJob struct {
	ID           string     `json:"id"`
	TemplateID   string     `json:"templateId"`
	TemplateName string     `json:"templateName"`
	DeviceID     string     `json:"deviceId"`
	DeviceName   string     `json:"deviceName"`
	SiteID       string     `json:"siteId"`
	Automatic    bool       `json:"automatic"`
	State        string     `json:"state"`
	Output       string     `json:"output"`
	Error        string     `json:"error"`
	QueuedAt     time.Time  `json:"queuedAt"`
	StartedAt    *time.Time `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
}
type ActionPayload struct {
	JobID          string   `json:"jobId"`
	Executable     string   `json:"executable"`
	Arguments      []string `json:"arguments"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	ExpiresAt      int64    `json:"expiresAt"`
}

func validateTemplate(v ActionTemplate) error {
	if v.Name == "" || !filepath.IsAbs(v.Executable) {
		return errors.New("name and absolute executable are required")
	}
	blocked := map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "fish": true, "python": true, "python3": true, "perl": true, "ruby": true, "node": true}
	if blocked[filepath.Base(v.Executable)] {
		return errors.New("shells and general interpreters are not permitted remediation executables")
	}
	if len(v.Arguments) > 32 {
		return errors.New("at most 32 arguments are allowed")
	}
	for _, arg := range v.Arguments {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("arguments cannot contain NUL")
		}
	}
	return nil
}
func (s *Store) ActionTemplates(ctx context.Context) ([]ActionTemplate, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,executable,arguments,timeout_seconds,coalesce(auto_check_type,''),enabled FROM action_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActionTemplate{}
	for rows.Next() {
		var v ActionTemplate
		var raw []byte
		if err = rows.Scan(&v.ID, &v.Name, &v.Executable, &raw, &v.TimeoutSeconds, &v.AutoCheckType, &v.Enabled); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v.Arguments)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateActionTemplate(ctx context.Context, v ActionTemplate, userID string) (ActionTemplate, error) {
	if err := validateTemplate(v); err != nil {
		return v, err
	}
	if v.TimeoutSeconds == 0 {
		v.TimeoutSeconds = 30
	}
	raw, _ := json.Marshal(v.Arguments)
	v.Enabled = true
	err := s.Pool.QueryRow(ctx, `INSERT INTO action_templates(name,executable,arguments,timeout_seconds,auto_check_type,created_by) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) RETURNING id,enabled`, v.Name, v.Executable, raw, v.TimeoutSeconds, v.AutoCheckType, userID).Scan(&v.ID, &v.Enabled)
	return v, err
}
func (s *Store) DeleteActionTemplate(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE action_templates SET enabled=false WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("template not found")
	}
	return err
}
func (s *Store) RemediationEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.Pool.QueryRow(ctx, `SELECT enabled FROM remediation_settings WHERE id=true`).Scan(&enabled)
	return enabled, err
}
func (s *Store) SetRemediationEnabled(ctx context.Context, enabled bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE remediation_settings SET enabled=$1 WHERE id=true`, enabled)
	if !enabled {
		_, _ = s.Pool.Exec(ctx, `UPDATE remediation_jobs SET state='cancelled',finished_at=now(),error='global remediation kill switch activated' WHERE state='queued'`)
	}
	return err
}
func (s *Store) QueueRemediation(ctx context.Context, templateID, deviceID, userID string, automatic bool) (RemediationJob, error) {
	enabled, err := s.RemediationEnabled(ctx)
	if err != nil {
		return RemediationJob{}, err
	}
	if !enabled {
		return RemediationJob{}, errors.New("remediation is disabled")
	}
	var recent bool
	if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM remediation_jobs WHERE template_id=$1 AND device_id=$2 AND queued_at>now()-interval '10 minutes')`, templateID, deviceID).Scan(&recent); err != nil {
		return RemediationJob{}, err
	}
	if recent {
		return RemediationJob{}, errors.New("remediation cooldown is active")
	}
	var v RemediationJob
	err = s.Pool.QueryRow(ctx, `INSERT INTO remediation_jobs(template_id,device_id,requested_by,automatic) SELECT $1,$2,NULLIF($3,'')::uuid,$4 FROM action_templates WHERE id=$1 AND enabled RETURNING id,template_id,device_id,automatic,state,queued_at`, templateID, deviceID, userID, automatic).Scan(&v.ID, &v.TemplateID, &v.DeviceID, &v.Automatic, &v.State, &v.QueuedAt)
	return v, err
}
func (s *Store) QueueAutomaticRemediation(ctx context.Context, deviceID, checkType string) {
	enabled, _ := s.RemediationEnabled(ctx)
	if !enabled {
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM action_templates WHERE enabled AND auto_check_type=$1`, checkType)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			_, _ = s.QueueRemediation(ctx, id, deviceID, "", true)
		}
	}
}
func (s *Store) RemediationJobs(ctx context.Context) ([]RemediationJob, error) {
	rows, err := s.Pool.Query(ctx, `SELECT j.id,j.template_id,t.name,j.device_id,d.name,coalesce(d.site_id::text,''),j.automatic,j.state,j.output,j.error,j.queued_at,j.started_at,j.finished_at FROM remediation_jobs j JOIN action_templates t ON t.id=j.template_id JOIN devices d ON d.id=j.device_id ORDER BY j.queued_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemediationJob{}
	for rows.Next() {
		var v RemediationJob
		if err = rows.Scan(&v.ID, &v.TemplateID, &v.TemplateName, &v.DeviceID, &v.DeviceName, &v.SiteID, &v.Automatic, &v.State, &v.Output, &v.Error, &v.QueuedAt, &v.StartedAt, &v.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) RequireAgent(ctx context.Context, serial string) (Identity, error) {
	v, err := s.IdentityBySerial(ctx, serial)
	if err != nil {
		return v, err
	}
	if v.Kind != "agent" {
		return v, errors.New("agent identity required")
	}
	s.TouchIdentity(ctx, serial)
	return v, nil
}
func (s *Store) ClaimAgentAction(ctx context.Context, identityID string) (ActionPayload, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ActionPayload{}, err
	}
	defer tx.Rollback(ctx)
	var p ActionPayload
	var args []byte
	err = tx.QueryRow(ctx, `SELECT j.id,t.executable,t.arguments,t.timeout_seconds FROM remediation_jobs j JOIN action_templates t ON t.id=j.template_id JOIN agent_reports ar ON ar.device_id=j.device_id WHERE ar.identity_id=$1 AND j.state='queued' AND t.enabled AND (SELECT enabled FROM remediation_settings WHERE id=true) ORDER BY j.queued_at FOR UPDATE OF j SKIP LOCKED LIMIT 1`, identityID).Scan(&p.JobID, &p.Executable, &args, &p.TimeoutSeconds)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(args, &p.Arguments)
	p.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	_, err = tx.Exec(ctx, `UPDATE remediation_jobs SET state='running',started_at=now() WHERE id=$1`, p.JobID)
	if err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}
func (s *Store) CompleteAgentAction(ctx context.Context, identityID, jobID string, success bool, output, message string) error {
	state := "failed"
	if success {
		state = "succeeded"
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE remediation_jobs j SET state=$3,output=$4,error=$5,finished_at=now() FROM agent_reports ar WHERE j.id=$1 AND ar.identity_id=$2 AND ar.device_id=j.device_id AND j.state='running'`, jobID, identityID, state, output, message)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("running remediation job not found")
	}
	return err
}
