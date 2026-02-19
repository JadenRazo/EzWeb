package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"strconv"

	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
)

func ExportSitesCSV(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("export sites failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Export failed")
		}

		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", "attachment; filename=sites.csv")

		w := csv.NewWriter(c.Response().BodyWriter())
		w.Write([]string{"ID", "Domain", "Container", "Port", "Status", "SSL", "Local", "Created"})

		for _, s := range sites {
			w.Write([]string{
				strconv.Itoa(s.ID),
				s.Domain,
				s.ContainerName,
				strconv.Itoa(s.Port),
				s.Status,
				strconv.FormatBool(s.SSLEnabled),
				strconv.FormatBool(s.IsLocal),
				s.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		w.Flush()
		return nil
	}
}

func ExportCustomersCSV(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("export customers failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Export failed")
		}

		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", "attachment; filename=customers.csv")

		w := csv.NewWriter(c.Response().BodyWriter())
		w.Write([]string{"ID", "Name", "Email", "Phone", "Company", "Created"})

		for _, cu := range customers {
			w.Write([]string{
				strconv.Itoa(cu.ID),
				cu.Name,
				cu.Email,
				cu.Phone,
				cu.Company,
				cu.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		w.Flush()
		return nil
	}
}

func ExportPaymentsCSV(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		payments, err := models.GetAllPayments(db)
		if err != nil {
			log.Printf("export payments failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Export failed")
		}

		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", "attachment; filename=payments.csv")

		w := csv.NewWriter(c.Response().BodyWriter())
		w.Write([]string{"ID", "Customer", "Site", "Amount", "Due Date", "Status", "Paid At", "Notes", "Created"})

		for _, p := range payments {
			paidAt := ""
			if p.PaidAt.Valid {
				paidAt = p.PaidAt.Time.Format("2006-01-02 15:04:05")
			}
			w.Write([]string{
				strconv.Itoa(p.ID),
				p.CustomerName,
				p.SiteDomain,
				fmt.Sprintf("%.2f", p.Amount),
				p.DueDate,
				p.Status,
				paidAt,
				p.Notes,
				p.CreatedAt,
			})
		}
		w.Flush()
		return nil
	}
}
