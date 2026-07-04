package repository

import (
	"icarus-auth-ms/internal/models"
	"database/sql"
	"errors"
	"time"
)

// CreateClient inserts or updates a client.
func (r *SQLRepository) CreateClient(clientID, publicKey string) error {
	if r.driver == "postgres" {
		_, err := r.db.Exec(`
			INSERT INTO clients (client_id, public_key) 
			VALUES ($1, $2) 
			ON CONFLICT (client_id) 
			DO UPDATE SET public_key = EXCLUDED.public_key`,
			clientID, publicKey)
		return err
	}
	_, err := r.db.Exec(`INSERT OR REPLACE INTO clients (client_id, public_key) VALUES (?, ?)`, clientID, publicKey)
	return err
}

// GetClientByID retrieves a client.
func (r *SQLRepository) GetClientByID(clientID string) (*models.Client, error) {
	var c models.Client
	if r.driver == "postgres" {
		row := r.db.QueryRow(`SELECT client_id, public_key, created_at FROM clients WHERE client_id = $1`, clientID)
		err := row.Scan(&c.ClientID, &c.PublicKey, &c.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("client not found")
			}
			return nil, err
		}
		return &c, nil
	}

	row := r.db.QueryRow(`SELECT client_id, public_key, created_at FROM clients WHERE client_id = ?`, clientID)

	var createdAtStr string
	err := row.Scan(&c.ClientID, &c.PublicKey, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("client not found")
		}
		return nil, err
	}

	c.CreatedAt, err = parseTime(createdAtStr)
	if err != nil {
		c.CreatedAt = time.Now()
	}

	return &c, nil
}

// GetClientPublicKey retrieves just the public key PEM for a client (satisfies crypto.ClientPublicKeyProvider).
func (r *SQLRepository) GetClientPublicKey(clientID string) (string, error) {
	client, err := r.GetClientByID(clientID)
	if err != nil {
		return "", err
	}
	return client.PublicKey, nil
}
