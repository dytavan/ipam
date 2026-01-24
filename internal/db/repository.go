package db

import (
	"ipam/internal/models"
	"log"
	"time"
)

// GetAllRacks retrieves all racks
func GetAllRacks() ([]models.Rack, error) {
	rows, err := DB.Query("SELECT id, name, location, height, status, created_at FROM racks ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var racks []models.Rack
	for rows.Next() {
		var r models.Rack
		if err := rows.Scan(&r.ID, &r.Name, &r.Location, &r.Height, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		racks = append(racks, r)
	}
	return racks, nil
}

// AddRack adds a new rack
func AddRack(r models.Rack) error {
	status := r.Status
	if status == "" {
		status = "Online"
	}
	_, err := DB.Exec("INSERT INTO racks (name, location, height, status, created_at) VALUES (?, ?, ?, ?, ?)",
		r.Name, r.Location, r.Height, status, time.Now())
	return err
}

// GetRack retrieves a single rack by ID
func GetRack(id int) (models.Rack, error) {
	var r models.Rack
	err := DB.QueryRow("SELECT id, name, location, height, status, created_at FROM racks WHERE id = ?", id).
		Scan(&r.ID, &r.Name, &r.Location, &r.Height, &r.Status, &r.CreatedAt)
	return r, err
}

// UpdateRack updates an existing rack
func UpdateRack(r models.Rack) error {
	_, err := DB.Exec("UPDATE racks SET name=?, location=?, height=?, status=? WHERE id=?",
		r.Name, r.Location, r.Height, r.Status, r.ID)
	return err
}

// DeleteRack deletes a rack
func DeleteRack(id int) error {
	// Optional: Handle devices associated with this rack (Set rack_id = 0 or CASCADE)
	// For now, let's set them to 0 (Unassigned) manually or rely on DB constraints if set (ON DELETE SET NULL)
	// Our CREATE TABLE had: FOREIGN KEY(rack_id) REFERENCES racks(id) ON DELETE SET NULL
	// So just deleting the rack should work fine.

	_, err := DB.Exec("DELETE FROM racks WHERE id=?", id)
	return err
}

// GetAllDevices retrieves all devices and their interfaces
// JOINs with racks table to get rack name
func GetAllDevices() ([]models.Device, error) {
	query := `
		SELECT d.id, d.hostname, d.device_type, d.rack_id, COALESCE(r.name, '') as rack_name, d.status, d.description, d.updated_at 
		FROM devices d 
		LEFT JOIN racks r ON d.rack_id = r.id 
		ORDER BY d.updated_at DESC`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.Hostname, &d.DeviceType, &d.RackID, &d.RackName, &d.Status, &d.Description, &d.UpdatedAt); err != nil {
			return nil, err
		}

		// Fetch interfaces for this device
		ifaces, err := GetDeviceInterfaces(d.ID)
		if err != nil {
			log.Printf("Error fetching interfaces for device %d: %v", d.ID, err)
			continue
		}
		d.Interfaces = ifaces

		devices = append(devices, d)
	}
	return devices, nil
}

// GetDeviceInterfaces retrieves interfaces for a specific device ID
func GetDeviceInterfaces(deviceID int) ([]models.DeviceInterface, error) {
	rows, err := DB.Query("SELECT id, device_id, ip_address, mac_address, label FROM device_interfaces WHERE device_id = ?", deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ifaces []models.DeviceInterface
	for rows.Next() {
		var i models.DeviceInterface
		if err := rows.Scan(&i.ID, &i.DeviceID, &i.IPAddress, &i.MACAddress, &i.Label); err != nil {
			return nil, err
		}
		ifaces = append(ifaces, i)
	}
	return ifaces, nil
}

// GetDevice retrieves a single device by ID with its interfaces
func GetDevice(id int) (models.Device, error) {
	var d models.Device
	query := `
		SELECT d.id, d.hostname, d.device_type, d.rack_id, COALESCE(r.name, '') as rack_name, d.status, d.description, d.updated_at 
		FROM devices d 
		LEFT JOIN racks r ON d.rack_id = r.id 
		WHERE d.id = ?`

	err := DB.QueryRow(query, id).
		Scan(&d.ID, &d.Hostname, &d.DeviceType, &d.RackID, &d.RackName, &d.Status, &d.Description, &d.UpdatedAt)
	if err != nil {
		return d, err
	}

	d.Interfaces, err = GetDeviceInterfaces(d.ID)
	return d, err
}

// AddDevice adds a new device and its interfaces
func AddDevice(d models.Device) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}

	result, err := tx.Exec("INSERT INTO devices (hostname, device_type, rack_id, status, description, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		d.Hostname, d.DeviceType, d.RackID, d.Status, d.Description, time.Now())
	if err != nil {
		tx.Rollback()
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return err
	}

	for _, iface := range d.Interfaces {
		_, err := tx.Exec("INSERT INTO device_interfaces (device_id, ip_address, mac_address, label) VALUES (?, ?, ?, ?)",
			id, iface.IPAddress, iface.MACAddress, iface.Label)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// UpdateDevice updates an existing device and replaces its interfaces
func UpdateDevice(d models.Device) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE devices SET hostname=?, device_type=?, rack_id=?, status=?, description=?, updated_at=? WHERE id=?",
		d.Hostname, d.DeviceType, d.RackID, d.Status, d.Description, time.Now(), d.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Remove old interfaces
	_, err = tx.Exec("DELETE FROM device_interfaces WHERE device_id=?", d.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Insert new interfaces
	for _, iface := range d.Interfaces {
		_, err := tx.Exec("INSERT INTO device_interfaces (device_id, ip_address, mac_address, label) VALUES (?, ?, ?, ?)",
			d.ID, iface.IPAddress, iface.MACAddress, iface.Label)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// DeleteDevice deletes a device and its interfaces (manual cascade)
func DeleteDevice(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM device_interfaces WHERE device_id=?", id)
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec("DELETE FROM devices WHERE id=?", id)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
