.PHONY: help build run test test-v test-coverage clean migrate migrate-up migrate-down lint fmt dev install deps

# Default target
.DEFAULT_GOAL := help

# Binary name
BINARY_NAME=tierify
MAIN_PATH=./cmd/tierify

# Database
DB_PATH ?= tierify.db
MIGRATION_PATH=./internal/db/sqlite/migrations

# Colors for output
BLUE=\033[0;34m
GREEN=\033[0;32m
YELLOW=\033[0;33m
NC=\033[0m # No Color

help: ## Show this help message
	@echo '$(BLUE)Tierify - Flexible Limit Engine$(NC)'
	@echo ''
	@echo 'Usage:'
	@echo '  $(YELLOW)make$(NC) $(GREEN)<target>$(NC)'
	@echo ''
	@echo 'Targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}'

deps: ## Install dependencies
	@echo "$(BLUE)Installing dependencies...$(NC)"
	go mod download
	go mod tidy

install: ## Install development tools
	@echo "$(BLUE)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest

build: ## Build the binary
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)✓ Build complete: ./$(BINARY_NAME)$(NC)"

run: build ## Build and run the server
	@echo "$(BLUE)Starting Tierify server...$(NC)"
	./$(BINARY_NAME)

dev: ## Run in development mode with hot reload (requires air)
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "$(YELLOW)air not found. Install with: go install github.com/cosmtrek/air@latest$(NC)"; \
		echo "$(BLUE)Falling back to normal run...$(NC)"; \
		$(MAKE) run; \
	fi

test: ## Run all tests
	@echo "$(BLUE)Running tests...$(NC)"
	go test ./...

test-v: ## Run all tests with verbose output
	@echo "$(BLUE)Running tests (verbose)...$(NC)"
	go test -v ./...

test-coverage: ## Run tests with coverage report
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

test-watch: ## Run tests in watch mode (requires gotestsum)
	@if command -v gotestsum > /dev/null; then \
		gotestsum --watch -- ./...; \
	else \
		echo "$(YELLOW)gotestsum not found. Install with: go install gotest.tools/gotestsum@latest$(NC)"; \
	fi

lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not found. Install with: make install$(NC)"; \
	fi

fmt: ## Format code
	@echo "$(BLUE)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

vet: ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	go vet ./...

clean: ## Clean build artifacts and test caches
	@echo "$(BLUE)Cleaning...$(NC)"
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -f $(DB_PATH)
	@echo "$(GREEN)✓ Clean complete$(NC)"

migrate: ## Show migration commands
	@echo "$(BLUE)Migration commands:$(NC)"
	@echo "  $(GREEN)make migrate-up$(NC)    - Apply all migrations"
	@echo "  $(GREEN)make migrate-down$(NC)  - Rollback last migration"
	@echo "  $(GREEN)make migrate-reset$(NC) - Reset database (down all + up all)"

migrate-up: ## Apply database migrations
	@if command -v migrate > /dev/null; then \
		echo "$(BLUE)Applying migrations...$(NC)"; \
		migrate -path $(MIGRATION_PATH) -database "sqlite3://$(DB_PATH)" up; \
		echo "$(GREEN)✓ Migrations applied$(NC)"; \
	else \
		echo "$(YELLOW)migrate not found. Install with: make install$(NC)"; \
	fi

migrate-down: ## Rollback last migration
	@if command -v migrate > /dev/null; then \
		echo "$(BLUE)Rolling back last migration...$(NC)"; \
		migrate -path $(MIGRATION_PATH) -database "sqlite3://$(DB_PATH)" down 1; \
		echo "$(GREEN)✓ Migration rolled back$(NC)"; \
	else \
		echo "$(YELLOW)migrate not found. Install with: make install$(NC)"; \
	fi

migrate-reset: ## Reset database (all down + all up)
	@if command -v migrate > /dev/null; then \
		echo "$(BLUE)Resetting database...$(NC)"; \
		migrate -path $(MIGRATION_PATH) -database "sqlite3://$(DB_PATH)" down -all; \
		migrate -path $(MIGRATION_PATH) -database "sqlite3://$(DB_PATH)" up; \
		echo "$(GREEN)✓ Database reset$(NC)"; \
	else \
		echo "$(YELLOW)migrate not found. Install with: make install$(NC)"; \
	fi

check: fmt vet lint test ## Run all checks (format, vet, lint, test)
	@echo "$(GREEN)✓ All checks passed$(NC)"

quick: fmt test ## Quick check (format + test)
	@echo "$(GREEN)✓ Quick check passed$(NC)"

.PHONY: docker-build docker-run docker-stop
docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t tierify:latest .
	@echo "$(GREEN)✓ Docker image built$(NC)"

docker-run: ## Run in Docker container
	@echo "$(BLUE)Running Docker container...$(NC)"
	docker run -p 8080:8080 --rm tierify:latest

docker-stop: ## Stop Docker container
	@echo "$(BLUE)Stopping Docker containers...$(NC)"
	docker stop $$(docker ps -q --filter ancestor=tierify:latest)
