package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Customer struct {
	ID        int
	Name      string
	Email     string
	Phone     string
	Company   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func GetAllCustomers(db *sql.DB) ([]Customer, error) {
	rows, err := db.Query("SELECT id, name, COALESCE(email,''), COALESCE(phone,''), COALESCE(company,''), created_at, updated_at FROM customers ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan customer: %w", err)
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

func GetCustomerByID(db *sql.DB, id int) (*Customer, error) {
	c := &Customer{}
	err := db.QueryRow(
		"SELECT id, name, COALESCE(email,''), COALESCE(phone,''), COALESCE(company,''), created_at, updated_at FROM customers WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("customer not found: %w", err)
	}
	return c, nil
}

func CreateCustomer(db *sql.DB, c *Customer) error {
	result, err := db.Exec(
		"INSERT INTO customers (name, email, phone, company) VALUES (?, ?, ?, ?)",
		c.Name, c.Email, c.Phone, c.Company,
	)
	if err != nil {
		return fmt.Errorf("failed to create customer: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	c.ID = int(id)
	return nil
}

func UpdateCustomer(db *sql.DB, c *Customer) error {
	_, err := db.Exec(
		"UPDATE customers SET name = ?, email = ?, phone = ?, company = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Name, c.Email, c.Phone, c.Company, c.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}
	return nil
}

func DeleteCustomer(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM customers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete customer: %w", err)
	}
	return nil
}

func CountCustomers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count customers: %w", err)
	}
	return count, nil
}

func GetCustomersPaginated(db *sql.DB, limit, offset int) ([]Customer, error) {
	rows, err := db.Query(
		"SELECT id, name, COALESCE(email,''), COALESCE(phone,''), COALESCE(company,''), created_at, updated_at FROM customers ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query customers: %w", err)
	}
	defer rows.Close()

	var customers []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan customer: %w", err)
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}
