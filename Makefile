.PHONY: build run dev test clean docker docker-run

# Variables
BINARY_NAME=netdiagram
DOCKER_IMAGE=netdiagram
VERSION?=latest

# Build the application
build:
	CGO_ENABLED=1 go build -o $(BINARY_NAME) ./cmd/server

# Run the application
run: build
	./$(BINARY_NAME) -addr :3000

# Run with file watching (development)
dev: build
	./$(BINARY_NAME) -addr :3000 -yaml ../ansible/infrastructure.yml

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f *.db *.db-journal *.db-wal *.db-shm

# Build Docker image
docker:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

# Run Docker container
docker-run:
	docker run -d \
		--name netdiagram \
		-p 3000:3000 \
		-v $$(pwd)/data:/data \
		$(DOCKER_IMAGE):$(VERSION)

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o $(BINARY_NAME)-linux-amd64 ./cmd/server
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -o $(BINARY_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -o $(BINARY_NAME)-darwin-arm64 ./cmd/server

# Tidy dependencies
tidy:
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run
