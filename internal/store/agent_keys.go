package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) ListAgentAPIKeys(ctx context.Context) ([]AgentAPIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, key_prefix, created_at, last_used_at, revoked_at
FROM agent_api_keys
ORDER BY datetime(created_at) DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []AgentAPIKey{}
	for rows.Next() {
		var item AgentAPIKey
		var createdAt sql.NullString
		var lastUsedAt sql.NullString
		var revokedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.KeyPrefix, &createdAt, &lastUsedAt, &revokedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = parseDBTime(createdAt)
		if value := parseDBTime(lastUsedAt); !value.IsZero() {
			item.LastUsedAt = &value
		}
		if value := parseDBTime(revokedAt); !value.IsZero() {
			item.RevokedAt = &value
		}
		keys = append(keys, item)
	}
	return keys, rows.Err()
}

func (s *Store) CreateAgentAPIKey(ctx context.Context, name string) (AgentAPIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Agent Key"
	}
	rawKey, err := generateOpaqueSecret("sbag_", 24)
	if err != nil {
		return AgentAPIKey{}, "", err
	}
	keyPrefix := rawKey
	if len(keyPrefix) > 18 {
		keyPrefix = keyPrefix[:18]
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO agent_api_keys (name, key_prefix, key_hash)
VALUES (?, ?, ?)
`, name, keyPrefix, hashSecretValue(rawKey))
	if err != nil {
		return AgentAPIKey{}, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AgentAPIKey{}, "", err
	}
	key := AgentAPIKey{
		ID:        id,
		Name:      name,
		KeyPrefix: keyPrefix,
		CreatedAt: time.Now(),
	}
	return key, rawKey, nil
}

func (s *Store) RevokeAgentAPIKey(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE agent_api_keys
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE id = ?
`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AuthenticateAgentAPIKey(ctx context.Context, rawKey string) (AgentAPIKey, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return AgentAPIKey{}, ErrInvalidAgentKey
	}

	var item AgentAPIKey
	var createdAt sql.NullString
	var lastUsedAt sql.NullString
	var revokedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, key_prefix, created_at, last_used_at, revoked_at
FROM agent_api_keys
WHERE key_hash = ?
`, hashSecretValue(rawKey)).Scan(&item.ID, &item.Name, &item.KeyPrefix, &createdAt, &lastUsedAt, &revokedAt)
	if errorsIsNoRows(err) {
		return AgentAPIKey{}, ErrInvalidAgentKey
	}
	if err != nil {
		return AgentAPIKey{}, err
	}
	item.CreatedAt = parseDBTime(createdAt)
	if value := parseDBTime(lastUsedAt); !value.IsZero() {
		item.LastUsedAt = &value
	}
	if value := parseDBTime(revokedAt); !value.IsZero() {
		item.RevokedAt = &value
	}
	if item.RevokedAt != nil {
		return AgentAPIKey{}, ErrInvalidAgentKey
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE agent_api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, item.ID)
	now := time.Now()
	item.LastUsedAt = &now
	return item, nil
}
