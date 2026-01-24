package models

import "time"

// DeviceInterface represents a network interface for a device
type DeviceInterface struct {
	ID         int    `json:"id"`
	DeviceID   int    `json:"device_id"`
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address"`
	Label      string `json:"label"` // e.g. "LAN", "WAN", "Management"
}

// Rack represents a physical equipment rack
type Rack struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	Height    int       `json:"height"` // in U
	Status    string    `json:"status"` // "Online", "Offline"
	CreatedAt time.Time `json:"created_at"`
}

// Device represents a network device in the IPAM system
type Device struct {
	ID          int               `json:"id"`
	Hostname    string            `json:"hostname"`
	DeviceType  string            `json:"device_type"`
	RackID      int               `json:"rack_id"`   // Foreign Key
	RackName    string            `json:"rack_name"` // Display purpose (from JOIN)
	Status      string            `json:"status"`    // e.g., "Online", "Offline", "Reserved"
	Description string            `json:"description"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Interfaces  []DeviceInterface `json:"interfaces"` // One-to-many relationship
}
