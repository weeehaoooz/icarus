package repository

import (
	"icarus-auth-ms/internal/models"
	"database/sql"
	"errors"
	"time"
)

// StoreRefreshToken inserts a new refresh token.
func (r *SQLRepository) StoreRefreshToken(token string, userID int64, expiresAt time.Time) error {
	if r.driver == "postgres" {
		_, err := r.db.Exec(`INSERT INTO refresh_tokens (token, user_id, expires_at) VALUES ($1, $2, $3)`,
			token, userID, expiresAt)
		return err
	}
	_, err := r.db.Exec(`INSERT INTO refresh_tokens (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, expiresAt.Format(time.RFC3339))
	return err
}

// GetRefreshToken retrieves a refresh token.
func (r *SQLRepository) GetRefreshToken(token string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	if r.driver == "postgres" {
		row := r.db.QueryRow(`SELECT token, user_id, expires_at, created_at FROM refresh_tokens WHERE token = $1`, token)
		err := row.Scan(&t.Token, &t.UserID, &t.ExpiresAt, &t.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("refresh token not found")
			}
			return nil, err
		}
		return &t, nil
	}

	row := r.db.QueryRow(`SELECT token, user_id, expires_at, created_at FROM refresh_tokens WHERE token = ?`, token)

	var expiresAtStr, createdAtStr string
	err := row.Scan(&t.Token, &t.UserID, &expiresAtStr, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("refresh token not found")
		}
		return nil, err
	}

	t.ExpiresAt, err = time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		t.ExpiresAt, err = parseTime(expiresAtStr)
		if err != nil {
			return nil, err
		}
	}

	t.CreatedAt, err = parseTime(createdAtStr)
	if err != nil {
		t.CreatedAt = time.Now()
	}

	return &t, nil
}

// DeleteRefreshToken deletes a refresh token.
func (r *SQLRepository) DeleteRefreshToken(token string) error {
	if r.driver == "postgres" {
		_, err := r.db.Exec(`DELETE FROM refresh_tokens WHERE token = $1`, token)
		return err
	}
	_, err := r.db.Exec(`DELETE FROM refresh_tokens WHERE token = ?`, token)
	return err
}
