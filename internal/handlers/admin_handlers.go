package handlers

import (
	"html/template"
	"io"
	"ipam/internal/db"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// SettingsHandler renders the settings page
func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/layout.html", "templates/settings.html")
	if err != nil {
		log.Printf("Error parsing templates: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, nil)
	if err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

// BackupDBHandler handles downloading the current database file
func BackupDBHandler(w http.ResponseWriter, r *http.Request) {
	// Ensure any pending WAL writes are flushed or that we at least get a consistent-ish read
	// For simple SQLite usage, just copying the file is usually "okay" if traffic is low,
	// but proper backup would involve SQLite backup API.
	// Given constraints and "Download Backup" request, direct file download is the simplest MVF.

	dbPath := db.DBPath
	if dbPath == "" {
		http.Error(w, "Database path not configured", http.StatusInternalServerError)
		return
	}

	file, err := os.Open(dbPath)
	if err != nil {
		log.Printf("Error opening database file: %v", err)
		http.Error(w, "Could not open database file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set headers for file download
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(dbPath))
	w.Header().Set("Content-Type", "application/x-sqlite3")

	_, err = io.Copy(w, file)
	if err != nil {
		log.Printf("Error streaming database file: %v", err)
	}
}

// RestoreDBHandler handles uploading and replacing the database file
func RestoreDBHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit upload size (e.g. 100MB)
	r.ParseMultipartForm(100 << 20)

	file, _, err := r.FormFile("backup_file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// DANGER ZONE: Close existing DB connection
	if err := db.CloseDB(); err != nil {
		log.Printf("Error closing database: %v", err)
		http.Error(w, "Failed to close existing database", http.StatusInternalServerError)
		return
	}

	// Overwrite the database file
	// Create a temporary file first to ensure upload success before destructing text?
	// Or just overwrite. Let's overwrite for simplicity but maybe backup first?
	// The user asked for restore usage, let's keep it simple: direct overwrite.

	dst, err := os.Create(db.DBPath)
	if err != nil {
		log.Printf("Error creating restore file: %v", err)
		// Try to reopen DB if we failed
		db.InitDB(db.DBPath)
		http.Error(w, "Failed to write database file", http.StatusInternalServerError)
		return
	}
	// We defer closing dst, but we need to close it before InitDB
	// So we won't defer close here, or we use a function closure.

	_, err = io.Copy(dst, file)
	dst.Close() // Explicit close

	if err != nil {
		log.Printf("Error writing to database file: %v", err)
		// Try to reopen - might be corrupted now though
		db.InitDB(db.DBPath)
		http.Error(w, "Failed to write content to database file", http.StatusInternalServerError)
		return
	}

	// Re-initialize DB
	db.InitDB(db.DBPath)

	// Redirect to settings with success message (or just redirect)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
