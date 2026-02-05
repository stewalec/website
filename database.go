package main

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func (app *App) initDB() error {
	var err error
	// pragmas:
	// - journal_mode=WAL: enable write-ahead log for concurrency & performance
	// - foreign_keys=ON: need foreign keys
	// - busy_timeout=5000: lock 5 seconds
	// - synchronous=NORMAL: "The synchronous=NORMAL setting is a good choice for most applications running in WAL mode."
	// - cache_size=-64000: 64MB ram for db cache
	app.db, err = sql.Open("sqlite", "website.db?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)")
	if err != nil {
		return err
	}

	// Create schema_migrations table if it doesn't exist
	_, err = app.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (app *App) runMigrations() error {
	var latestVersion int
	err := app.db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&latestVersion)
	if err != nil {
		if strings.Contains(err.Error(), "converting NULL to int is unsupported") {
			// assume that we're starting from ground zero
			latestVersion = 0
		} else {
			return err
		}
	}

	log.Printf("Current schema version: %d", latestVersion)

	files, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	for _, f := range files {
		var version int
		_, err = fmt.Sscanf(f.Name(), "%d_", &version)
		if err != nil {
			return err
		}

		// Apply migration if not already applied
		if version > latestVersion {
			fileData, _ := fs.ReadFile(migrationFiles, "migrations/"+f.Name())
			_, err := app.db.Exec(string(fileData))
			if err != nil {
				return fmt.Errorf("Failed to apply migration %s: %v", f.Name(), err)
			}
			_, err = app.db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version)
			if err != nil {
				return fmt.Errorf("Failed to record migration version %d: %v", version, err)
			}
			log.Printf("Applied migration %s\n", f.Name())
		}
	}

	return nil
}

func (app *App) createInitialUser() error {
	var count int
	err := app.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		var username string
		log.Print("No users found. Prompting to create a user\n")

		fmt.Print("Username: ")
		fmt.Scanln(&username)

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			fmt.Printf("\nError reading password: %v\n", err)
			return err
		}
		fmt.Println()

		hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		_, err = app.db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", username, string(hashedPassword))
		if err != nil {
			return err
		}
		log.Printf("User %s created", username)
	}

	return nil
}
