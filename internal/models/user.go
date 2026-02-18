package models

import (
	"database/sql"
	"fmt"
	"time"
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

func EnsureAdminExists(db *sql.DB, username, hashedPassword string) error {
	var exists int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check admin existence: %w", err)
	}
	if exists == 0 {
		return CreateUser(db, username, hashedPassword)
	}
	return nil
}
