# Makefile for IPAM

BINARY_NAME=ipam
DOCKER_IMAGE=davidyannick/ipam
PORT=8080

.PHONY: all build build-mac build-linux run clean docker-build docker-run

all: build

# Build for local architecture
build:
	go build -o bin/$(BINARY_NAME) main.go

# Build for Mac ARM (M1/M2/M3)
build-mac:
	GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-mac main.go

# Build for Linux AMD64
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux main.go

# Run locally
run:
	go run main.go

# Docker Build (Local arch)
docker-build:
	docker build -t $(DOCKER_IMAGE) .

# Docker Run (with DB persistence and env vars)
docker-run:
	docker run -p $(PORT):8080 -v $(PWD)/data:/data $(DOCKER_IMAGE)

# Multi-arch Docker Build (requires docker buildx)
docker-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE) .

clean:
	rm -rf bin/
