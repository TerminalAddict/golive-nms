package store

import (
	"context"
	"encoding/json"
	"errors"
)

type Credential struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`
	Secret map[string]string `json:"secret,omitempty"`
}

func (s *Store) Credentials(ctx context.Context) ([]Credential, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,kind FROM credentials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Credential{}
	for rows.Next() {
		var c Credential
		if err = rows.Scan(&c.ID, &c.Name, &c.Kind); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) CreateCredential(ctx context.Context, c Credential) (Credential, error) {
	if c.Kind != "snmp" && c.Kind != "ssh" && c.Kind != "smtp" && c.Kind != "webhook" && c.Kind != "routeros" && c.Kind != "monit" {
		return Credential{}, errors.New("invalid credential kind")
	}
	raw, err := json.Marshal(c.Secret)
	if err != nil {
		return Credential{}, err
	}
	encrypted, err := s.Vault.Seal(raw)
	if err != nil {
		return Credential{}, err
	}
	err = s.Pool.QueryRow(ctx, `INSERT INTO credentials(name,kind,encrypted_data) VALUES($1,$2,$3) RETURNING id`, c.Name, c.Kind, encrypted).Scan(&c.ID)
	c.Secret = nil
	return c, err
}
func (s *Store) CredentialSecret(ctx context.Context, id string) (Credential, error) {
	var c Credential
	var encrypted []byte
	err := s.Pool.QueryRow(ctx, `SELECT id,name,kind,encrypted_data FROM credentials WHERE id=$1`, id).Scan(&c.ID, &c.Name, &c.Kind, &encrypted)
	if err != nil {
		return c, err
	}
	raw, err := s.Vault.Open(encrypted)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c.Secret)
	return c, err
}
func (s *Store) DeleteCredential(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM credentials WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("credential not found")
	}
	return err
}
