package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Enrollment struct {
	ID, Kind, SiteID, Name string
	ExpiresAt              time.Time
}
type Identity struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	SiteID     string     `json:"siteId"`
	Name       string     `json:"name"`
	Serial     string     `json:"serial"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
}

func (s *Store) LoadCA(ctx context.Context) (cert, key []byte, err error) {
	var encrypted []byte
	err = s.Pool.QueryRow(ctx, `SELECT certificate_pem,encrypted_private_key FROM pki_authority WHERE id=true`).Scan(&cert, &encrypted)
	if err != nil {
		return
	}
	key, err = s.Vault.Open(encrypted)
	return
}
func (s *Store) SaveCA(ctx context.Context, cert, key []byte) error {
	encrypted, err := s.Vault.Seal(key)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO pki_authority(id,certificate_pem,encrypted_private_key) VALUES(true,$1,$2) ON CONFLICT(id) DO NOTHING`, cert, encrypted)
	return err
}
func (s *Store) CreateEnrollment(ctx context.Context, kind, siteID, userID string, ttl time.Duration) (string, time.Time, error) {
	if kind != "agent" && kind != "collector" {
		return "", time.Time{}, errors.New("invalid enrollment kind")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := "gle_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().Add(ttl)
	_, err := s.Pool.Exec(ctx, `INSERT INTO enrollment_tokens(token_hash,kind,site_id,created_by,expires_at) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5)`, sum[:], kind, siteID, userID, expires)
	return token, expires, err
}
func (s *Store) ConsumeEnrollment(ctx context.Context, plain, name string) (Enrollment, error) {
	sum := sha256.Sum256([]byte(plain))
	var e Enrollment
	err := s.Pool.QueryRow(ctx, `UPDATE enrollment_tokens SET used_at=now() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>now() RETURNING id,kind,coalesce(site_id::text,''),expires_at`, sum[:]).Scan(&e.ID, &e.Kind, &e.SiteID, &e.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, errors.New("invalid, expired, or already-used enrollment token")
	}
	e.Name = name
	return e, err
}
func (s *Store) SaveIdentity(ctx context.Context, e Enrollment, serial string, cert []byte, expires time.Time) (Identity, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer tx.Rollback(ctx)
	var v Identity
	err = tx.QueryRow(ctx, `INSERT INTO enrolled_identities(kind,site_id,name,serial,certificate_pem,expires_at) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,$6) RETURNING id,kind,coalesce(site_id::text,''),name,serial,expires_at,revoked_at,last_seen_at`, e.Kind, e.SiteID, e.Name, serial, cert, expires).Scan(&v.ID, &v.Kind, &v.SiteID, &v.Name, &v.Serial, &v.ExpiresAt, &v.RevokedAt, &v.LastSeenAt)
	if err != nil {
		return v, err
	}
	if e.Kind == "collector" && e.SiteID != "" {
		if _, err = tx.Exec(ctx, `UPDATE sites SET collector_identity_id=$1 WHERE id=$2`, v.ID, e.SiteID); err != nil {
			return v, err
		}
	}
	return v, tx.Commit(ctx)
}
func (s *Store) Identities(ctx context.Context) ([]Identity, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,kind,coalesce(site_id::text,''),name,serial,expires_at,revoked_at,last_seen_at FROM enrolled_identities ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Identity{}
	for rows.Next() {
		var v Identity
		if err = rows.Scan(&v.ID, &v.Kind, &v.SiteID, &v.Name, &v.Serial, &v.ExpiresAt, &v.RevokedAt, &v.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) RevokeIdentity(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE enrolled_identities SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("active identity not found")
	}
	return err
}
func (s *Store) IdentityActive(ctx context.Context, serial string) bool {
	var active bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrolled_identities WHERE serial=$1 AND revoked_at IS NULL AND expires_at>now())`, serial).Scan(&active)
	return active
}
func (s *Store) TouchIdentity(ctx context.Context, serial string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE enrolled_identities SET last_seen_at=now() WHERE serial=$1`, serial)
}
func (s *Store) IdentityBySerial(ctx context.Context, serial string) (Identity, error) {
	var v Identity
	err := s.Pool.QueryRow(ctx, `SELECT id,kind,coalesce(site_id::text,''),name,serial,expires_at,revoked_at,last_seen_at FROM enrolled_identities WHERE serial=$1 AND revoked_at IS NULL AND expires_at>now()`, serial).Scan(&v.ID, &v.Kind, &v.SiteID, &v.Name, &v.Serial, &v.ExpiresAt, &v.RevokedAt, &v.LastSeenAt)
	return v, err
}
