# Build Stage
FROM golang:1.25.6 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
# CGO_ENABLED=1 is required for go-sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o ipam .

# Run Stage
# Use debian slim to have package manager access for ping
FROM debian:bookworm-slim

# Install ping
RUN apt-get update && apt-get install -y iputils-ping && rm -rf /var/lib/apt/lists/*

WORKDIR /

# Copy the Pre-built binary from the previous stage
COPY --from=builder /app/ipam /ipam

# Copy static files and templates
COPY --from=builder /app/static /static
COPY --from=builder /app/templates /templates

# Create directory for DB
WORKDIR /data
# Copy DB if it exists (glob hack for optional copy)
COPY --from=builder /app/ipam.d* /data/


WORKDIR /
ENV IP_RANGE_START=192.168.1
ENV PORT=8080

# Expose port
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/ipam"]
CMD ["/data/ipam.db"]
