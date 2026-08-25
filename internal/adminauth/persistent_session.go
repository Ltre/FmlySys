package adminauth

import (
	"context"
	"database/sql"
	"time"
)

// PersistentSessionTTL is intentionally much longer than the old 12-hour
// administrator session. Session rows live in system.db, so a normal server
// restart does not invalidate them.
const PersistentSessionTTL = 30 * 24 * time.Hour

func (s *Service) BeginPersistentSession(ctx context.Context, userID int64, stage string) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	nowUTC := time.Now().UTC()
	_, err = s.DB.ExecContext(
		ctx,
		`INSERT INTO admin_sessions(token_hash,admin_user_id,stage,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?)`,
		hash,
		userID,
		stage,
		nowUTC.Add(PersistentSessionTTL).Format(time.RFC3339Nano),
		nowUTC.Format(time.RFC3339Nano),
		nowUTC.Format(time.RFC3339Nano),
	)
	return raw, err
}

// ExtendPersistentSession implements a sliding 30-day expiry. It deliberately
// requires the row to still be unexpired; an already expired cookie cannot
// resurrect an administrator session.
func (s *Service) ExtendPersistentSession(ctx context.Context, raw string) error {
	if raw == "" {
		return sql.ErrNoRows
	}
	hash := tokenHash(raw)
	nowUTC := time.Now().UTC()
	res, err := s.DB.ExecContext(
		ctx,
		`UPDATE admin_sessions
SET expires_at=?,last_seen_at=?
WHERE token_hash=? AND expires_at>?`,
		nowUTC.Add(PersistentSessionTTL).Format(time.RFC3339Nano),
		nowUTC.Format(time.RFC3339Nano),
		hash,
		nowUTC.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
