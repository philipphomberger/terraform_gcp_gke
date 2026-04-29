.PHONY: dev build run test lint docker-up docker-down migrate-up migrate-down migrate-create

# Start development environment
docker-up:
	docker compose up -d

docker-down:
	docker compose down

# Run the Go server locally (infra via docker)
dev: docker-up
	go run ./cmd/server

# Build binary
build:
	go build -o bin/hombox ./cmd/server

# Run tests
test:
	go test ./... -v -race

# Lint
lint:
	golangci-lint run ./...

# Database migrations
migrate-up:
	go run ./cmd/server --migrate-up

migrate-down:
	go run ./cmd/server --migrate-down

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir internal/database/migrations -seq $$name

# Frontend
web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
