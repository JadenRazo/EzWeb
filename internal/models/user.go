package models

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int
	Username  string
	Password  string
	CreatedAt time.Time
}

func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		"SELECT id, username, password, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}

func CreateUser(db *sql.DB, username, hashedPassword string) error {
	_, err := db.Exec(
		"INSERT INTO users (username, password) VALUES (?, ?)",
		username, hashedPassword,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// EnsureAdminExists creates the admin user if it doesn't exist, or updates
// the stored password hash when the configured plain-text password no longer
// matches — so changes to ADMIN_PASS in .env take effect on restart.
func EnsureAdminExists(db *sql.DB, username, plainPassword string) error {
	var currentHash string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&currentHash)

	if err == sql.ErrNoRows {
		// User doesn't exist yet — create it.
		hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}
		return CreateUser(db, username, string(hash))
	}
	if err != nil {
		return fmt.Errorf("failed to check admin existence: %w", err)
	}

	// User exists — check whether the stored hash still matches the configured password.
	if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(plainPassword)) != nil {
		// Hash mismatch: the .env password was changed, so re-hash and update.
		hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash updated admin password: %w", err)
		}
		_, err = db.Exec("UPDATE users SET password = ? WHERE username = ?", string(hash), username)
		if err != nil {
			return fmt.Errorf("failed to update admin password: %w", err)
		}
	}

	return nil
}
