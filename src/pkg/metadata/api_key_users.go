package metadata

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
)

type APIKeyUser struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeySuffix  string  `json:"key_suffix,omitempty"`
	Enabled    bool    `json:"enabled"`
	CreatedAt  string  `json:"created_at,omitempty"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
}

func (s *Store) CreateAPIKeyUser(ctx context.Context, name string) (*APIKeyUser, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("name 不能为空")
	}
	rawSecret, err := securitypkg.GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}
	apiKey := "blgo_" + rawSecret
	user := &APIKeyUser{
		ID:        newAPIKeyUserID(),
		Name:      name,
		KeySuffix: keySuffix(apiKey),
		Enabled:   true,
	}

	s.mu.Lock()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_key_users (id, name, key_hash, key_suffix, enabled)
		VALUES (?, ?, ?, ?, 1)
	`, user.ID, user.Name, securitypkg.HashAPIKey(apiKey), user.KeySuffix)
	s.mu.Unlock()
	if err != nil {
		return nil, "", err
	}
	created, err := s.GetAPIKeyUserByID(ctx, user.ID)
	if err != nil {
		return user, apiKey, nil
	}
	return created, apiKey, nil
}

func (s *Store) ListAPIKeyUsers(ctx context.Context) ([]APIKeyUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, key_suffix, enabled, created_at, last_used_at, revoked_at
		FROM api_key_users
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []APIKeyUser
	for rows.Next() {
		user, err := scanAPIKeyUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

func (s *Store) GetAPIKeyUserByID(ctx context.Context, id string) (*APIKeyUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, key_suffix, enabled, created_at, last_used_at, revoked_at
		FROM api_key_users WHERE id = ?
	`, id)
	return scanAPIKeyUser(row)
}

func (s *Store) FindActiveAPIKeyUserByKey(ctx context.Context, apiKey string) (*APIKeyUser, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, sql.ErrNoRows
	}
	s.mu.RLock()
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, key_suffix, enabled, created_at, last_used_at, revoked_at
		FROM api_key_users
		WHERE key_hash = ? AND enabled = 1 AND revoked_at IS NULL
	`, securitypkg.HashAPIKey(apiKey))
	user, err := scanAPIKeyUser(row)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	_ = s.TouchAPIKeyUser(ctx, user.ID)
	return user, nil
}

func (s *Store) FindActiveAPIKeyUserBySignedRequest(ctx context.Context, validate func(secret string) bool) (*APIKeyUser, error) {
	s.mu.RLock()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, key_suffix, enabled, created_at, last_used_at, revoked_at, key_hash
		FROM api_key_users
		WHERE enabled = 1 AND revoked_at IS NULL
	`)
	if err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var user APIKeyUser
		var enabled int
		var lastUsedAt, revokedAt sql.NullString
		var keyHash string
		if err := rows.Scan(&user.ID, &user.Name, &user.KeySuffix, &enabled, &user.CreatedAt, &lastUsedAt, &revokedAt, &keyHash); err != nil {
			s.mu.RUnlock()
			return nil, err
		}
		if !validate(keyHash) {
			continue
		}
		user.Enabled = enabled == 1
		if lastUsedAt.Valid {
			user.LastUsedAt = &lastUsedAt.String
		}
		if revokedAt.Valid {
			user.RevokedAt = &revokedAt.String
		}
		s.mu.RUnlock()
		_ = s.TouchAPIKeyUser(ctx, user.ID)
		return &user, nil
	}
	if err := rows.Err(); err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	s.mu.RUnlock()
	return nil, sql.ErrNoRows
}

func (s *Store) HasAPIKeyUsers(ctx context.Context) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM api_key_users WHERE revoked_at IS NULL").Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func (s *Store) UpdateAPIKeyUser(ctx context.Context, id string, name *string, enabled *bool) (*APIKeyUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, fmt.Errorf("name 不能为空")
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE api_key_users SET name = ? WHERE id = ? AND revoked_at IS NULL", trimmed, id); err != nil {
			return nil, err
		}
	}
	if enabled != nil {
		enabledInt := 0
		if *enabled {
			enabledInt = 1
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE api_key_users SET enabled = ? WHERE id = ? AND revoked_at IS NULL", enabledInt, id); err != nil {
			return nil, err
		}
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, key_suffix, enabled, created_at, last_used_at, revoked_at
		FROM api_key_users WHERE id = ?
	`, id)
	return scanAPIKeyUser(row)
}

func (s *Store) RevokeAPIKeyUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		UPDATE api_key_users
		SET enabled = 0, revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND revoked_at IS NULL
	`, id)
	return err
}

func (s *Store) TouchAPIKeyUser(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "UPDATE api_key_users SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
}

type apiKeyUserScanner interface {
	Scan(dest ...any) error
}

func scanAPIKeyUser(row apiKeyUserScanner) (*APIKeyUser, error) {
	var user APIKeyUser
	var enabled int
	var lastUsedAt, revokedAt sql.NullString
	if err := row.Scan(&user.ID, &user.Name, &user.KeySuffix, &enabled, &user.CreatedAt, &lastUsedAt, &revokedAt); err != nil {
		return nil, err
	}
	user.Enabled = enabled == 1
	if lastUsedAt.Valid {
		user.LastUsedAt = &lastUsedAt.String
	}
	if revokedAt.Valid {
		user.RevokedAt = &revokedAt.String
	}
	return &user, nil
}

func newAPIKeyUserID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "usr_fallback"
	}
	return "usr_" + hex.EncodeToString(buf[:])
}

func keySuffix(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if len(apiKey) <= 4 {
		return strings.ToUpper(apiKey)
	}
	return strings.ToUpper(apiKey[len(apiKey)-4:])
}
