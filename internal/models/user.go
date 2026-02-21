package models

import (
	"database/sql"
	"fmt"
	"time"

	"ezweb/internal/auth"
)

type User struct {
	ID          int
	Username    string
	Password    string
	Role        string
	TOTPSecret  string
	TOTPEnabled bool
	CreatedAt   time.Time
}

func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	user := &User{}
	var totpSecret sql.NullString
	err := db.QueryRow(
		"SELECT id, username, password, COALESCE(role, 'admin'), COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Role, &totpSecret, &user.TOTPEnabled, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	user.TOTPSecret = totpSecret.String
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

func GetAllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, username, COALESCE(role, 'admin'), created_at FROM users ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func CreateUserWithRole(db *sql.DB, username, hashedPassword, role string) error {
	_, err := db.Exec(
		"INSERT INTO users (username, password, role) VALUES (?, ?, ?)",
		username, hashedPassword, role,
	)
	return err
}

func UpdateUserRole(db *sql.DB, id int, role string) error {
	_, err := db.Exec("UPDATE users SET role = ? WHERE id = ?", role, id)
	return err
}

func GetUserByID(db *sql.DB, id int) (*User, error) {
	user := &User{}
	var totpSecret sql.NullString
	err := db.QueryRow(
		"SELECT id, username, password, COALESCE(role, 'admin'), COALESCE(totp_secret, ''), COALESCE(totp_enabled, 0), created_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Role, &totpSecret, &user.TOTPEnabled, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	user.TOTPSecret = totpSecret.String
	return user, nil
}

func UpdateUserPassword(db *sql.DB, id int, hashedPassword string) error {
	_, err := db.Exec("UPDATE users SET password = ? WHERE id = ?", hashedPassword, id)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

func DeleteUser(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func SetTOTPSecret(db *sql.DB, userID int, secret string) error {
	_, err := db.Exec("UPDATE users SET totp_secret = ? WHERE id = ?", secret, userID)
	return err
}

func EnableTOTP(db *sql.DB, userID int) error {
	_, err := db.Exec("UPDATE users SET totp_enabled = 1 WHERE id = ?", userID)
	return err
}

func DisableTOTP(db *sql.DB, userID int) error {
	_, err := db.Exec("UPDATE users SET totp_enabled = 0, totp_secret = NULL WHERE id = ?", userID)
	return err
}

func IsTOTPCodeUsed(db *sql.DB, userID int, code string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM totp_used_codes WHERE user_id = ? AND code = ?",
		userID, code,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func RecordTOTPCodeUsed(db *sql.DB, userID int, code string) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO totp_used_codes (user_id, code) VALUES (?, ?)",
		userID, code,
	)
	return err
}

func CleanupUsedTOTPCodes(db *sql.DB) {
	db.Exec("DELETE FROM totp_used_codes WHERE used_at < ?", time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339))
}

// EnsureAdminExists creates the admin user if it doesn't exist, or updates
// the stored password hash when the configured plain-text password no longer
// matches — so changes to ADMIN_PASS in .env take effect on restart.
func EnsureAdminExists(db *sql.DB, username, plainPassword string) error {
	var currentHash string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&currentHash)

	if err == sql.ErrNoRows {
		// User doesn't exist yet — create it.
		hash, err := auth.HashPassword(plainPassword)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}
		return CreateUser(db, username, hash)
	}
	if err != nil {
		return fmt.Errorf("failed to check admin existence: %w", err)
	}

	// User exists — check whether the stored hash still matches the configured password.
	if !auth.CheckPassword(currentHash, plainPassword) {
		// Hash mismatch: the .env password was changed, so re-hash and update.
		hash, err := auth.HashPassword(plainPassword)
		if err != nil {
			return fmt.Errorf("failed to hash updated admin password: %w", err)
		}
		_, err = db.Exec("UPDATE users SET password = ? WHERE username = ?", hash, username)
		if err != nil {
			return fmt.Errorf("failed to update admin password: %w", err)
		}
	}

	return nil
}
