package main

import (
	"ipam/internal/db"
	"ipam/internal/handlers"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	// Initialize Database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		if len(os.Args) > 1 {
			dbPath = os.Args[1]
		} else {
			dbPath = "ipam.db"
		}
	}
	db.InitDB(dbPath)
	defer db.DB.Close()

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Routes
	http.HandleFunc("/", handlers.HomeHandler)
	http.HandleFunc("/add", handlers.AddDeviceHandler)
	http.HandleFunc("/create", handlers.CreateDeviceHandler)
	http.HandleFunc("/edit", handlers.EditDeviceHandler)
	http.HandleFunc("/update", handlers.UpdateDeviceHandler)
	http.HandleFunc("/delete", handlers.DeleteDeviceHandler)

	http.HandleFunc("/add-rack", handlers.AddRackHandler)
	http.HandleFunc("/create-rack", handlers.CreateRackHandler)
	http.HandleFunc("/edit-rack", handlers.EditRackHandler)
	http.HandleFunc("/update-rack", handlers.UpdateRackHandler)
	http.HandleFunc("/delete-rack", handlers.DeleteRackHandler)

	http.HandleFunc("/ping", handlers.PingDeviceHandler)
	http.HandleFunc("/export/csv", handlers.ExportCSVHandler)
	http.HandleFunc("/export/excel", handlers.ExportExcelHandler)
	http.HandleFunc("/scan", handlers.ScanSubnetHandler)

	// Admin / Settings
	http.HandleFunc("/settings", handlers.SettingsHandler)
	http.HandleFunc("/backup", handlers.BackupDBHandler)
	http.HandleFunc("/restore", handlers.RestoreDBHandler)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server started at http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
