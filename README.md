[![Docker image CI](https://github.com/dytavan/ipam/actions/workflows/publish-image.yaml/badge.svg)](https://github.com/dytavan/ipam/actions/workflows/publish-image.yaml)

# IPAM (IP Address Management)

A lightweight and efficient IP Address Management (IPAM) system built with **Go** and **SQLite**. This application helps network administrators manage racks, devices, and IP allocations within their infrastructure through a clean web interface.

## Features

*   **Dashboard Overview**: Visual representation of IP usage statistics and rack organization.
*   **Rack Management**: Organize your infrastructure by creating and managing physical racks (Location, Height, Status).
*   **Device Inventory**: Track network devices including Hostname, Type, Status, and Description.
*   **Multi-Interface Support**: Assign multiple network interfaces (IP, MAC, Label) to a single device.
*   **Visual IP Map**: Interactive grid showing allocation status (Free, Used, Reserved) for the active subnet.
*   **Network Tools**:
    *   **Subnet Scan**: Concurrently scan the network to identify active IP addresses using ICMP pings.
    *   **Device Ping**: Check connectivity of specific devices directly from the UI.
*   **Data Export**: Export your device inventory to **CSV** and **Excel** formats.
*   **Smart Sorting**: Sort devices by IP address or Hostname.

## Tech Stack

*   **Backend**: Go (Golang)
*   **Database**: SQLite
*   **Frontend**: HTML Templates (Server-Side Rendering)
*   **Key Libraries**:
    *   `github.com/mattn/go-sqlite3`: SQLite driver.
    *   `github.com/xuri/excelize/v2`: Excel file generation.

## Getting Started

### Prerequisites

*   Go 1.18 or higher.
*   GCC (required for CGO/SQLite).

### Installation

1.  Clone the repository.
2.  Install dependencies:
    ```bash
    go mod tidy
    ```
3.  Run the application:
    ```bash
    go run main.go
    ```

### Configuration

You can configure the target subnet range by setting the `IP_RANGE_START` environment variable (default is `192.168.1`).
