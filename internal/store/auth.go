package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}
type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	Token      string     `json:"token,omitempty"`
}

var ErrInvalidCredentials = errors.New("invalid email or password")

func (s *Store) EnsureAdmin(ctx context.Context, email, password string) error {
	if email == "" || password == "" {
		return errors.New("administrator email and password are required")
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role='administrator')`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO users(email,display_name,password_hash,role) VALUES($1,'Administrator',$2,'administrator')`, strings.ToLower(strings.TrimSpace(email)), string(hash))
	return err
}
func (s *Store) Login(ctx context.Context, email, password string) (User, string, error) {
	var u User
	var hash string
	err := s.Pool.QueryRow(ctx, `SELECT id,email,display_name,role,password_hash FROM users WHERE email=$1 AND active`, strings.ToLower(strings.TrimSpace(email))).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, "", ErrInvalidCredentials
	}
	token, err := s.CreateSession(ctx, u.ID)
	if err == nil {
		_ = s.Audit(ctx, u.ID, "login", "session", "")
	}
	return u, token, err
}
func (s *Store) CreateSession(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '12 hours')`, sum[:], userID)
	return token, err
}
func (s *Store) UpsertOIDCUser(ctx context.Context, email, name string) (User, error) {
	if name == "" {
		name = email
	}
	var u User
	err := s.Pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role) VALUES($1,$2,'','viewer') ON CONFLICT(email) DO UPDATE SET display_name=excluded.display_name,active=true,updated_at=now() RETURNING id,email,display_name,role`, strings.ToLower(strings.TrimSpace(email)), name).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	return u, err
}
func (s *Store) SessionUser(ctx context.Context, token string) (User, error) {
	sum := sha256.Sum256([]byte(token))
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT u.id,u.email,u.display_name,u.role FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active`, sum[:]).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	return u, err
}
func (s *Store) Logout(ctx context.Context, token string) error {
	sum := sha256.Sum256([]byte(token))
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, sum[:])
	return err
}
func (s *Store) Audit(ctx context.Context, userID, action, resourceType, resourceID string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO audit_log(user_id,action,resource_type,resource_id) VALUES(NULLIF($1,'')::uuid,$2,$3,$4)`, userID, action, resourceType, resourceID)
	return err
}
func (s *Store) PruneSessions(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _ = s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`)
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,email,display_name,role FROM users WHERE active ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Store) CreateUser(ctx context.Context, email, name, password, role string) (User, error) {
	if role != "administrator" && role != "manager" && role != "site_manager" && role != "viewer" {
		return User{}, errors.New("invalid role")
	}
	if len(password) < 12 {
		return User{}, errors.New("password must be at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.Pool.QueryRow(ctx, `INSERT INTO users(email,display_name,password_hash,role) VALUES($1,$2,$3,$4) RETURNING id,email,display_name,role`, strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(name), string(hash), role).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	return u, err
}
func (s *Store) DeleteUser(ctx context.Context, id, actorID string) error {
	if id == actorID {
		return errors.New("cannot delete your own account")
	}
	var admins int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='administrator' AND active`).Scan(&admins); err != nil {
		return err
	}
	var role string
	if err := s.Pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, id).Scan(&role); err != nil {
		return err
	}
	if role == "administrator" && admins <= 1 {
		return errors.New("cannot delete the last administrator")
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET active=false,updated_at=now() WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("user not found")
	}
	return err
}
func (s *Store) UpdateUser(ctx context.Context, id, name, role, password string) (User, error) {
	if role != "administrator" && role != "manager" && role != "site_manager" && role != "viewer" {
		return User{}, errors.New("invalid role")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var oldRole string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 AND active`, id).Scan(&oldRole); err != nil {
		return User{}, err
	}
	if oldRole == "administrator" && role != "administrator" {
		var admins int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='administrator' AND active`).Scan(&admins); err != nil {
			return User{}, err
		}
		if admins <= 1 {
			return User{}, errors.New("cannot demote the last administrator")
		}
	}
	if name == "" {
		return User{}, errors.New("display name is required")
	}
	if password != "" {
		if len(password) < 12 {
			return User{}, errors.New("password must be at least 12 characters")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if e != nil {
			return User{}, e
		}
		_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2,role=$3,password_hash=$4,updated_at=now() WHERE id=$1`, id, name, role, string(hash))
	} else {
		_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2,role=$3,updated_at=now() WHERE id=$1`, id, name, role)
	}
	if err != nil {
		return User{}, err
	}
	var u User
	err = tx.QueryRow(ctx, `SELECT id,email,display_name,role FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	if err != nil {
		return u, err
	}
	return u, tx.Commit(ctx)
}
func (s *Store) CreateAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time) (APIToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APIToken{}, err
	}
	plain := "glv_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	var t APIToken
	err := s.Pool.QueryRow(ctx, `INSERT INTO api_tokens(user_id,name,token_hash,expires_at) VALUES($1,$2,$3,$4) RETURNING id,name,expires_at,last_used_at,created_at`, userID, strings.TrimSpace(name), sum[:], expiresAt).Scan(&t.ID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt)
	t.Token = plain
	return t, err
}
func (s *Store) APITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,expires_at,last_used_at,created_at FROM api_tokens WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err = rows.Scan(&t.ID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) TokenUser(ctx context.Context, plain string) (User, error) {
	sum := sha256.Sum256([]byte(plain))
	var u User
	err := s.Pool.QueryRow(ctx, `UPDATE api_tokens t SET last_used_at=now() FROM users u WHERE t.token_hash=$1 AND t.user_id=u.id AND u.active AND (t.expires_at IS NULL OR t.expires_at>now()) RETURNING u.id,u.email,u.display_name,u.role`, sum[:]).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role)
	return u, err
}
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM api_tokens WHERE id=$1 AND user_id=$2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("API token not found")
	}
	return err
}
