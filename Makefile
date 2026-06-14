.PHONY: run build test migrate-up migrate-down generate-models lint clean smoke-phase6

# Load .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Construct DATABASE_URL if not set
DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

# Go path for tools
GOBIN ?= $(shell go env GOPATH)/bin

# Development
init:
	go mod tidy

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

# Testing
test:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Database
migrate-up:
	$(GOBIN)/migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	$(GOBIN)/migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	$(GOBIN)/migrate create -ext sql -dir migrations -seq $(name)

migrate-force:
	$(GOBIN)/migrate -path migrations -database "$(DATABASE_URL)" force $(version)



# Linting
lint:
	golangci-lint run ./...

# Docker
docker-build:
	docker build -t payment-kita-backend:latest -f docker/Dockerfile .

docker-run:
	docker-compose -f docker/docker-compose.yml up -d

docker-stop:
	docker-compose -f docker/docker-compose.yml down

# Clean
clean:
	rm -rf bin/
	rm -rf coverage.out coverage.html

smoke-phase6:
	./scripts/phase6_smoke.sh
