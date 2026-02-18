package main

import (
	"log"
	"os"

	"ezweb/internal/db"
	mcptools "ezweb/internal/mcp"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	_ = godotenv.Load()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./ezweb.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	s := server.NewMCPServer(
		"ezweb",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	mcptools.RegisterTools(s, database)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
