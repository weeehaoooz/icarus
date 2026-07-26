package repository

import (
	"icarus-auth-ms/internal/models"
	"database/sql"
	"errors"
	"time"
)

// CreateUser inserts a user with active status defaulting to true.
func (r *SQLRepository) CreateUser(username, email, firstName, lastName, passwordHash string) (int64, error) {
	return r.CreateUserWithStatus(username, email, firstName, lastName, passwordHash, true)
}

// CreateUserWithStatus inserts a user with explicit active status.
func (r *SQLRepository) CreateUserWithStatus(username, email, firstName, lastName, passwordHash string, isActive bool) (int64, error) {
	if r.driver == "postgres" {
		var id int64
		err := r.db.QueryRow(`
			INSERT INTO users (username, email, first_name, last_name, password_hash, is_active) 
			VALUES ($1, $2, $3, $4, $5, $6) 
			RETURNING id`,
			username, email, firstName, lastName, passwordHash, isActive).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	result, err := r.db.Exec(`INSERT INTO users (username, email, first_name, last_name, password_hash, is_active) VALUES (?, ?, ?, ?, ?, ?)`, username, email, firstName, lastName, passwordHash, isActive)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetUserByUsername retrieves a user.
func (r *SQLRepository) GetUserByUsername(username string) (*models.User, error) {
	var u models.User
	var emailVal, firstNameVal, lastNameVal sql.NullString

	if r.driver == "postgres" {
		row := r.db.QueryRow(`SELECT id, username, email, first_name, last_name, password_hash, is_active, created_at FROM users WHERE username = $1`, username)
		err := row.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &u.PasswordHash, &u.IsActive, &u.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("user not found")
			}
			return nil, err
		}
		u.Email = emailVal.String
		u.FirstName = firstNameVal.String
		u.LastName = lastNameVal.String

		return &u, nil
	}

	row := r.db.QueryRow(`SELECT id, username, email, first_name, last_name, password_hash, is_active, created_at FROM users WHERE username = ?`, username)

	var createdAtStr string
	err := row.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &u.PasswordHash, &u.IsActive, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	u.Email = emailVal.String
	u.FirstName = firstNameVal.String
	u.LastName = lastNameVal.String

	u.CreatedAt, err = parseTime(createdAtStr)
	if err != nil {
		u.CreatedAt = time.Now()
	}

	return &u, nil
}

// GetUserByID retrieves a user by ID.
func (r *SQLRepository) GetUserByID(id int64) (*models.User, error) {
	var u models.User
	var emailVal, firstNameVal, lastNameVal sql.NullString

	if r.driver == "postgres" {
		row := r.db.QueryRow(`SELECT id, username, email, first_name, last_name, password_hash, is_active, created_at FROM users WHERE id = $1`, id)
		err := row.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &u.PasswordHash, &u.IsActive, &u.CreatedAt)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errors.New("user not found")
			}
			return nil, err
		}
		u.Email = emailVal.String
		u.FirstName = firstNameVal.String
		u.LastName = lastNameVal.String

		return &u, nil
	}

	row := r.db.QueryRow(`SELECT id, username, email, first_name, last_name, password_hash, is_active, created_at FROM users WHERE id = ?`, id)

	var createdAtStr string
	err := row.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &u.PasswordHash, &u.IsActive, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	u.Email = emailVal.String
	u.FirstName = firstNameVal.String
	u.LastName = lastNameVal.String

	u.CreatedAt, err = parseTime(createdAtStr)
	if err != nil {
		u.CreatedAt = time.Now()
	}

	return &u, nil
}
