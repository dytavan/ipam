package db

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var (
	DB     *sql.DB
	DBPath string
)

// InitDB initializes the SQLite database connection and creates tables
func InitDB(filepath string) {
	DBPath = filepath
	var err error
	DB, err = sql.Open("sqlite3", filepath)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	createTables()
}

// CloseDB closes the database connection
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

func createTables() {
	createRacksTable := `CREATE TABLE IF NOT EXISTS racks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		location TEXT,
		height INTEGER,
		created_at DATETIME
	);`

	if _, err := DB.Exec(createRacksTable); err != nil {
		log.Fatalf("Error creating racks table: %v", err)
	}

	createDevicesTable := `CREATE TABLE IF NOT EXISTS devices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hostname TEXT,
		device_type TEXT,
		rack_id INTEGER,
		status TEXT,
		description TEXT,
		updated_at DATETIME,
		FOREIGN KEY(rack_id) REFERENCES racks(id) ON DELETE SET NULL
	);`

	if _, err := DB.Exec(createDevicesTable); err != nil {
		log.Fatalf("Error creating devices table: %v", err)
	}

	// Migration for rack_id column
	_, err := DB.Exec("ALTER TABLE devices ADD COLUMN rack_id INTEGER DEFAULT 0")
	if err != nil {
		if err.Error() != "duplicate column name: rack_id" {
			// ignore
		}
	}

	// Migration for racks status column
	_, err = DB.Exec("ALTER TABLE racks ADD COLUMN status TEXT DEFAULT 'Online'")
	if err != nil {
		// Ignore if column exists
	}

	createInterfacesTable := `CREATE TABLE IF NOT EXISTS device_interfaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id INTEGER,
		ip_address TEXT,
		mac_address TEXT,
		label TEXT,
		FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE
	);`

	if _, err := DB.Exec(createInterfacesTable); err != nil {
		log.Fatalf("Error creating device_interfaces table: %v", err)
	}

	// Migration: Check if old columns exist in 'devices' (ip_address) and migrate data if needed.
	// For simplicity in this iteration, we will leave the old schema in place if it exists,
	// but the application will rely on the new table.
	// If this represents a fresh start or simple migration:
	// In a real app we would write a proper migration function.
}
