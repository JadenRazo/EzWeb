package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Server struct {
	ID         int
	Name       string
	Host       string
	SSHPort    int
	SSHUser    string
	SSHKeyPath string
	SSHHostKey string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func GetAllServers(db *sql.DB) ([]Server, error) {
	rows, err := db.Query(
		"SELECT id, name, host, ssh_port, ssh_user, ssh_key_path, COALESCE(ssh_host_key,''), status, created_at, updated_at FROM servers ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query servers: %w", err)
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.SSHPort, &s.SSHUser, &s.SSHKeyPath, &s.SSHHostKey, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan server row: %w", err)
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func GetServerByID(db *sql.DB, id int) (*Server, error) {
	s := &Server{}
	err := db.QueryRow(
		"SELECT id, name, host, ssh_port, ssh_user, ssh_key_path, COALESCE(ssh_host_key,''), status, created_at, updated_at FROM servers WHERE id = ?",
		id,
	).Scan(&s.ID, &s.Name, &s.Host, &s.SSHPort, &s.SSHUser, &s.SSHKeyPath, &s.SSHHostKey, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	return s, nil
}

func CreateServer(db *sql.DB, s *Server) error {
	result, err := db.Exec(
		"INSERT INTO servers (name, host, ssh_port, ssh_user, ssh_key_path, status) VALUES (?, ?, ?, ?, ?, ?)",
		s.Name, s.Host, s.SSHPort, s.SSHUser, s.SSHKeyPath, s.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	s.ID = int(id)
	return nil
}

func UpdateServer(db *sql.DB, s *Server) error {
	_, err := db.Exec(
		"UPDATE servers SET name = ?, host = ?, ssh_port = ?, ssh_user = ?, ssh_key_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		s.Name, s.Host, s.SSHPort, s.SSHUser, s.SSHKeyPath, s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}
	return nil
}

func DeleteServer(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}
	return nil
}

func UpdateServerHostKey(db *sql.DB, id int, hostKey string) error {
	_, err := db.Exec(
		"UPDATE servers SET ssh_host_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		hostKey, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update server host key: %w", err)
	}
	return nil
}

func UpdateServerStatus(db *sql.DB, id int, status string) error {
	_, err := db.Exec(
		"UPDATE servers SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update server status: %w", err)
	}
	return nil
}

func CountServers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM servers").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count servers: %w", err)
	}
	return count, nil
}
