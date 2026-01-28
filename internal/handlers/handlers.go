package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"ipam/internal/db"
	"ipam/internal/models"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// render parses the layout and the specific view template and executes the layout.
func render(w http.ResponseWriter, view string, data interface{}) {
	files := []string{
		filepath.Join("templates", "layout.html"),
		filepath.Join("templates", view),
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		log.Printf("Error parsing templates %v: %v", files, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = ts.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

type IPStatus struct {
	IP       string
	Octet    int
	Status   string // "Free", "Used", "Reserved"
	DeviceID int
	Hostname string
}

type RackGroup struct {
	Rack    models.Rack
	Devices []models.Device
}

type DashboardData struct {
	RackGroups        []RackGroup
	UnassignedDevices []models.Device
	Devices           []models.Device // Keep for global stats/logic if needed, or remove if unused in template (IPMap uses it)
	Racks             []models.Rack
	Subnet            string
	IPMap             []IPStatus
	TotalIPs          int
	UsedIPs           int
	FreeIPs           int
	UsagePercent      int
	Sort              string
	Order             string
}

// IP comparison helper
func compareIPs(ip1, ip2 string) bool {
	p1 := net.ParseIP(ip1)
	p2 := net.ParseIP(ip2)

	if p1 == nil || p2 == nil {
		return ip1 < ip2
	}

	// Convert to 4-byte representation for comparison (IPv4)
	p14 := p1.To4()
	p24 := p2.To4()

	if p14 == nil || p24 == nil {
		// Fallback for IPv6 or mixed
		return bytesCompare(p1, p2) < 0
	}

	return bytesCompare(p14, p24) < 0
}

func bytesCompare(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := db.GetAllDevices()
	if err != nil {
		http.Error(w, "Could not fetch devices", http.StatusInternalServerError)
		return
	}
	racks, err := db.GetAllRacks()
	if err != nil {
		log.Printf("Could not fetch racks: %v", err)
	}

	// Group devices by Rack
	// Map rack ID to devices
	rackDevicesMap := make(map[int][]models.Device)
	var unassignedDevices []models.Device

	for _, d := range devices {
		if d.RackID != 0 {
			rackDevicesMap[d.RackID] = append(rackDevicesMap[d.RackID], d)
		} else {
			unassignedDevices = append(unassignedDevices, d)
		}
	}

	// Create ordered groups based on Racks slice to maintain name order
	var rackGroups []RackGroup
	for _, rack := range racks {
		if devs, ok := rackDevicesMap[rack.ID]; ok {
			rackGroups = append(rackGroups, RackGroup{
				Rack:    rack,
				Devices: devs,
			})
		} else {
			// Show empty racks too? User said "name of each rack followed by machines".
			// Usually yes, show the rack even if empty.
			rackGroups = append(rackGroups, RackGroup{
				Rack:    rack,
				Devices: []models.Device{},
			})
		}
	}

	// Default sort params
	sortBy := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order")
	if sortOrder == "" {
		sortOrder = "asc"
	}

	// Sort devices in each rack
	for i := range rackGroups {
		sort.Slice(rackGroups[i].Devices, func(j, k int) bool {
			dev1 := rackGroups[i].Devices[j]
			dev2 := rackGroups[i].Devices[k]

			// Helper to get first IP
			getIP := func(d models.Device) string {
				if len(d.Interfaces) > 0 {
					return d.Interfaces[0].IPAddress
				}
				return ""
			}

			var less bool
			switch sortBy {
			case "ip":
				ip1 := getIP(dev1)
				ip2 := getIP(dev2)
				less = compareIPs(ip1, ip2)
			case "hostname":
				less = strings.ToLower(dev1.Hostname) < strings.ToLower(dev2.Hostname)
			default:
				// Default sort by ID or creation? Original was order from DB.
				// Let's default to hostname if nothing specified, or keep DB order?
				// User asked to add possibility to sort. If no sort param, maybe keep DB order?
				// But let's assume if they click, they send sort param.
				// If sortBy is empty, we do nothing here, maintaining DB order (which is usually ID).
				return dev1.ID < dev2.ID // Stable sort by ID if desired, or just return false to keep relative if stable?
				// sort.Slice is not stable. Let's just not sort if empty.
			}

			if sortOrder == "desc" {
				return !less
			}
			return less
		})
	}

	// Sort unassigned devices
	if len(unassignedDevices) > 0 {
		sort.Slice(unassignedDevices, func(j, k int) bool {
			dev1 := unassignedDevices[j]
			dev2 := unassignedDevices[k]

			// Helper to get first IP
			getIP := func(d models.Device) string {
				if len(d.Interfaces) > 0 {
					return d.Interfaces[0].IPAddress
				}
				return ""
			}

			var less bool
			switch sortBy {
			case "ip":
				ip1 := getIP(dev1)
				ip2 := getIP(dev2)
				less = compareIPs(ip1, ip2)
			case "hostname":
				less = strings.ToLower(dev1.Hostname) < strings.ToLower(dev2.Hostname)
			default:
				return dev1.ID < dev2.ID
			}

			if sortOrder == "desc" {
				return !less
			}
			return less
		})
	}

	// 1. Identify most common subnet (unchanged logic)
	subnetCounts := make(map[string]int)
	usedIPs := make(map[string]models.Device)
	usedIPsInterface := make(map[string]models.DeviceInterface)

	for _, d := range devices {
		for _, iface := range d.Interfaces {
			if iface.IPAddress == "" {
				continue
			}
			ipRaw := strings.ReplaceAll(iface.IPAddress, " ", "")
			parts := strings.Split(ipRaw, ".")
			if len(parts) == 4 {
				subnet := strings.Join(parts[:3], ".")
				subnetCounts[subnet]++
				usedIPs[ipRaw] = d
				usedIPsInterface[ipRaw] = iface
			}
		}
	}

	// Default subnet
	// Default subnet
	// Default subnet
	targetSubnet := os.Getenv("IP_RANGE_START")
	if targetSubnet == "" {
		targetSubnet = "192.168.1"
	}
	maxCount := 1 // Require at least 2 devices to override default
	for subnet, count := range subnetCounts {
		if count > maxCount {
			maxCount = count
			targetSubnet = subnet
		}
	}

	// 2. Build IP Map (1-254)
	var ipMap []IPStatus
	usedCount := 0

	for i := 1; i <= 254; i++ {
		ip := fmt.Sprintf("%s.%d", targetSubnet, i)
		status := "Free"
		deviceID := 0
		hostname := ""

		if device, exists := usedIPs[ip]; exists {
			deviceID = device.ID
			hostname = device.Hostname
			status = "Used"

			// Check if specifically reserved
			if device.Status == "Reserved" {
				status = "Reserved"
			}
			usedCount++
		}

		ipMap = append(ipMap, IPStatus{
			IP:       ip,
			Octet:    i,
			Status:   status,
			DeviceID: deviceID,
			Hostname: hostname,
		})
	}

	// Logic for free blocks removed

	data := DashboardData{
		RackGroups:        rackGroups,
		UnassignedDevices: unassignedDevices,
		Devices:           devices, // Passed for completeness, ensuring IPMap works if it relies on this? No, IPMap constructed above.
		Racks:             racks,
		Subnet:            targetSubnet,
		IPMap:             ipMap,
		TotalIPs:          254,
		UsedIPs:           usedCount,
		FreeIPs:           254 - usedCount,
		UsagePercent:      (usedCount * 100) / 254,
		Sort:              sortBy,
		Order:             sortOrder,
	}

	render(w, "index.html", data)
}

func AddDeviceHandler(w http.ResponseWriter, r *http.Request) {
	racks, err := db.GetAllRacks()
	if err != nil {
		http.Error(w, "Error fetching racks", http.StatusInternalServerError)
		return
	}

	// Check for pre-fill IP
	ipParam := r.URL.Query().Get("ip")
	var device models.Device
	if ipParam != "" {
		device.Interfaces = append(device.Interfaces, models.DeviceInterface{
			IPAddress: ipParam,
		})
	}

	data := struct {
		Device models.Device
		Racks  []models.Rack
	}{
		Device: device,
		Racks:  racks,
	}
	render(w, "form.html", data)
}

func AddRackHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "rack_form.html", nil)
}

func CreateRackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/add-rack", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	height, _ := strconv.Atoi(r.FormValue("height"))
	rack := models.Rack{
		Name:     r.FormValue("name"),
		Location: r.FormValue("location"),
		Height:   height,
		Status:   r.FormValue("status"),
	}

	if err := db.AddRack(rack); err != nil {
		log.Printf("Error adding rack: %v", err)
		http.Error(w, "Error adding rack", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func EditRackHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid rack ID", http.StatusBadRequest)
		return
	}

	rack, err := db.GetRack(id)
	if err != nil {
		http.Error(w, "Rack not found", http.StatusNotFound)
		return
	}

	render(w, "rack_form.html", rack)
}

func UpdateRackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	height, _ := strconv.Atoi(r.FormValue("height"))
	rack := models.Rack{
		ID:       id,
		Name:     r.FormValue("name"),
		Location: r.FormValue("location"),
		Height:   height,
		Status:   r.FormValue("status"),
	}

	if err := db.UpdateRack(rack); err != nil {
		log.Printf("Error updating rack: %v", err)
		http.Error(w, "Error updating rack", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func DeleteRackHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid rack ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteRack(id); err != nil {
		log.Printf("Error deleting rack: %v", err)
		http.Error(w, "Error deleting rack", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func CreateDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/add", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	rackID, _ := strconv.Atoi(r.FormValue("rack_id"))

	device := models.Device{
		Hostname:    r.FormValue("hostname"),
		DeviceType:  r.FormValue("device_type"),
		RackID:      rackID,
		Status:      r.FormValue("status"),
		Description: r.FormValue("description"),
	}

	// Parse Interfaces
	ips := r.PostForm["ip_address"]
	macs := r.PostForm["mac_address"]
	labels := r.PostForm["label"]

	// Ensure equal lengths or handle gracefully.
	// We assume the frontend sends matching arrays.
	count := len(ips)
	if count > 0 {
		for i := 0; i < count; i++ {
			if ips[i] == "" {
				continue
			}

			mac := ""
			if i < len(macs) {
				mac = macs[i]
			}

			label := ""
			if i < len(labels) {
				label = labels[i]
			}

			device.Interfaces = append(device.Interfaces, models.DeviceInterface{
				IPAddress:  strings.ReplaceAll(ips[i], " ", ""),
				MACAddress: strings.TrimSpace(mac),
				Label:      strings.TrimSpace(label),
			})
		}
	}

	if err := db.AddDevice(device); err != nil {
		log.Printf("Error adding device: %v", err)
		http.Error(w, "Error adding device", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func EditDeviceHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}

	device, err := db.GetDevice(id)
	if err != nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	racks, _ := db.GetAllRacks()

	data := struct {
		Device models.Device
		Racks  []models.Rack
	}{
		Device: device,
		Racks:  racks,
	}

	render(w, "form.html", data)
}

func UpdateDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}

	rackID, _ := strconv.Atoi(r.FormValue("rack_id"))

	device := models.Device{
		ID:          id,
		Hostname:    r.FormValue("hostname"),
		DeviceType:  r.FormValue("device_type"),
		RackID:      rackID,
		Status:      r.FormValue("status"),
		Description: r.FormValue("description"),
	}

	// Parse Interfaces
	ips := r.PostForm["ip_address"]
	macs := r.PostForm["mac_address"]
	labels := r.PostForm["label"]

	count := len(ips)
	if count > 0 {
		for i := 0; i < count; i++ {
			if ips[i] == "" {
				continue
			}

			mac := ""
			if i < len(macs) {
				mac = macs[i]
			}

			label := ""
			if i < len(labels) {
				label = labels[i]
			}

			device.Interfaces = append(device.Interfaces, models.DeviceInterface{
				IPAddress:  strings.ReplaceAll(ips[i], " ", ""),
				MACAddress: strings.TrimSpace(mac),
				Label:      strings.TrimSpace(label),
			})
		}
	}

	if err := db.UpdateDevice(device); err != nil {
		log.Printf("Error updating device: %v", err)
		http.Error(w, "Error updating device", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func DeleteDeviceHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteDevice(id); err != nil {
		log.Printf("Error deleting device: %v", err)
		http.Error(w, "Error deleting device", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// PingDeviceHandler pings the device and returns the result
func PingDeviceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	respond := func(success bool, output string) {
		json.NewEncoder(w).Encode(struct {
			Success bool   `json:"success"`
			Output  string `json:"output"`
		}{
			Success: success,
			Output:  output,
		})
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respond(false, "Invalid device ID")
		return
	}

	device, err := db.GetDevice(id)
	if err != nil {
		respond(false, "Device not found")
		return
	}

	requestedIP := r.URL.Query().Get("ip")
	targetIP := ""

	if len(device.Interfaces) == 0 {
		respond(false, "Device has no interfaces configured")
		return
	}

	if requestedIP != "" {
		// Verify this IP belongs to the device
		found := false
		for _, iface := range device.Interfaces {
			if iface.IPAddress == requestedIP {
				targetIP = requestedIP
				found = true
				break
			}
		}
		if !found {
			respond(false, "Requested IP does not belong to this device")
			return
		}
	} else {
		// Default to first IP
		targetIP = device.Interfaces[0].IPAddress
	}

	if targetIP == "" {
		respond(false, "No valid IP address found")
		return
	}

	// -c 3 for 3 packets
	cmd := exec.Command("ping", "-c", "3", targetIP)
	output, err := cmd.CombinedOutput()

	respond(err == nil, string(output))
}

// ExportCSVHandler exports devices to CSV
func ExportCSVHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := db.GetAllDevices()
	if err != nil {
		http.Error(w, "Could not fetch devices", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=devices.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	writer.Write([]string{"ID", "Hostname", "Type", "Rack", "Status", "IP Addresses", "MAC Addresses", "Description", "Last Updated"})

	for _, d := range devices {
		var ips, macs []string
		for _, iface := range d.Interfaces {
			ip := iface.IPAddress
			if iface.Label != "" {
				ip += " (" + iface.Label + ")"
			}
			ips = append(ips, ip)
			macs = append(macs, iface.MACAddress)
		}

		writer.Write([]string{
			strconv.Itoa(d.ID),
			d.Hostname,
			d.DeviceType,
			d.RackName,
			d.Status,
			strings.Join(ips, "; "),
			strings.Join(macs, "; "),
			d.Description,
			d.UpdatedAt.Format(time.RFC3339),
		})
	}
}

// ExportJSONHandler exports devices to JSON
func ExportJSONHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := db.GetAllDevices()
	if err != nil {
		http.Error(w, "Could not fetch devices", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=devices.json")

	if err := json.NewEncoder(w).Encode(devices); err != nil {
		log.Printf("Error encoding devices to JSON: %v", err)
		http.Error(w, "Error writing JSON file", http.StatusInternalServerError)
	}
}

// ScanSubnetHandler pings all IPs in the subnet and returns active ones
func ScanSubnetHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := db.GetAllDevices()
	if err != nil {
		http.Error(w, "Could not fetch devices", http.StatusInternalServerError)
		return
	}

	// Infer subnet (simplified logic from HomeHandler)
	subnetCounts := make(map[string]int)
	for _, d := range devices {
		for _, iface := range d.Interfaces {
			ipRaw := strings.ReplaceAll(iface.IPAddress, " ", "")
			parts := strings.Split(ipRaw, ".")
			if len(parts) == 4 {
				subnet := strings.Join(parts[:3], ".")
				subnetCounts[subnet]++
			}
		}
	}
	targetSubnet := os.Getenv("IP_RANGE_START")
	if targetSubnet == "" {
		targetSubnet = "192.168.1"
	}
	maxCount := 1 // Require at least 2 devices to override default
	for subnet, count := range subnetCounts {
		if count > maxCount {
			maxCount = count
			targetSubnet = subnet
		}
	}

	// Concurrent Scan
	var wg sync.WaitGroup
	var activeIPs []string
	var mutex sync.Mutex

	// Semaphore to limit concurrency (max 20 pings at once)
	sem := make(chan struct{}, 20)

	for i := 1; i <= 254; i++ {
		wg.Add(1)
		go func(octet int) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			ip := fmt.Sprintf("%s.%d", targetSubnet, octet)

			// Short timeout for scan: 1 count, 200-500ms timeout equivalent
			// On macOS ping -W is in milliseconds, on Linux it might be different.
			// Go's exec.Command context is cleaner for timeout.

			// Using exec with context for timeout
			// Linux/Mac common args: -c 1 -W 1 (1 second wait)
			// macOS uses -W in ms? No, -W wait time in ms on some BSDs, seconds on others.
			// On macOS 15: -W waittime (in milliseconds). standard ping is often -t for timeout?
			// Let's rely on Go Context timeout to kill the process.

			cmd := exec.Command("ping", "-c", "1", ip)
			// On macOS sending SIGKILL via context cancellation is safest to ensure speed.

			// Assuming network is fast, we allow 300ms.
			// Note: This might be too aggressive for some networks, but user wants speed.
			// Wait, let's just run it.

			// Actually, a simpler way for cross-platform is just `ping -c 1 -t 1` (BSD/Mac) or `-w 1` (Linux).
			// Let's stick to standard `ping -c 1` and let the OS handle the timeout naturally or use a wrapper.
			// But standard timeout is long (10s).
			// We MUST use a timeout.

			err := cmd.Start()
			if err != nil {
				return
			}

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			// 500ms Timeout
			select {
			case <-time.After(500 * time.Millisecond):
				if cmd.Process != nil {
					cmd.Process.Kill()
				}
			case err := <-done:
				if err == nil {
					mutex.Lock()
					activeIPs = append(activeIPs, ip)
					mutex.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Success   bool     `json:"success"`
		ActiveIPs []string `json:"active_ips"`
	}{
		Success:   true,
		ActiveIPs: activeIPs,
	})
}
