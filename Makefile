SHELL := /usr/bin/env bash

# ---- project config --------------------------------------------------
BINARY_NAME   := astro-backend
BUILD_DIR     := bin
CMD_PATH      := ./cmd
ENV_FILE      := .env
INFRA_COMPOSE := docker-compose.infra.yaml
DEV_COMPOSE   := docker-compose.dev.yaml
PROD_COMPOSE  := docker-compose.yaml
GOBIN         := $(shell go env GOPATH)/bin

# pinned tool versions
MOCKGEN_VERSION       := v0.6.0
# golangci-lint ставим через go install (собирается локальным go), версию
# фиксируем на latest — скрипт выполняется один раз через `make tools`.
GOLANGCI_LINT_VERSION := latest
GOOSE_VERSION         := v3.22.0

# ---- phony -----------------------------------------------------------
.PHONY: help tools tidy fmt vet \
        build run clean \
        test test-race test-integration cover \
        lint mocks mocks-clean generate \
        docker-build docker-up docker-down \
        local-up local-down local-logs \
        migrate-up migrate-down migrate-status \
        generate-key

.DEFAULT_GOAL := help

help: ## show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nAvailable targets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---- tooling ---------------------------------------------------------
tools: ## install development tools (mockgen, golangci-lint, goose)
	@echo ">> installing mockgen $(MOCKGEN_VERSION)"
	@go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)
	@echo ">> installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) || \
		(echo "go install failed, falling back to install script" && \
		 curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION))
	@echo ">> installing goose $(GOOSE_VERSION)"
	@go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) || true
	@echo "tools installed into $(GOBIN)"

tidy: ## go mod tidy
	go mod tidy

fmt: ## gofmt all sources
	gofmt -s -w .

vet: ## go vet
	go vet ./...

# ---- build / run -----------------------------------------------------
build: ## build server binary into bin/
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

run: ## run server locally (uses $(ENV_FILE))
	go run $(CMD_PATH)

clean: ## remove build artifacts and caches
	rm -rf $(BUILD_DIR) coverage.out

# ---- tests -----------------------------------------------------------
test: ## unit tests (without -race, fast)
	go test ./... -count=1

test-race: ## unit tests with race detector
	go test -race -count=1 ./...

test-integration: ## integration tests (requires Docker)
	go test -tags=integration -count=1 ./...

cover: ## tests + coverage summary
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# ---- lint / generate -------------------------------------------------
lint: ## run golangci-lint (use $GOBIN binary first, fallback to PATH)
	@bin="$(GOBIN)/golangci-lint"; \
	if [[ -x "$$bin" ]]; then \
		"$$bin" run ./...; \
	else \
		golangci-lint run ./...; \
	fi

mocks: ## regenerate all gomock mocks
	@PATH="$(GOBIN):$$PATH" go generate ./...

mocks-clean: ## remove generated mocks
	@find . -path '*/mocks/mock_*.go' -delete
	@echo "mocks removed"

generate: mocks ## alias: run every go generate

# ---- docker ---------------------------------------------------------
docker-build: ## build production docker image
	docker build -t $(BINARY_NAME):latest .

docker-up: ## start full stack (prod compose, app + infra in docker)
	docker compose -f $(PROD_COMPOSE) up -d --build

docker-down: ## stop full stack
	docker compose -f $(PROD_COMPOSE) down

infra-up: ## start only infra (postgres/nats/memcached/jaeger) for local dev
	docker compose -f $(INFRA_COMPOSE) up -d

infra-down: ## stop local infra
	docker compose -f $(INFRA_COMPOSE) down

infra-logs: ## tail local infra logs
	docker compose -f $(INFRA_COMPOSE) logs -f

dev-up: ## start local infra AND app with hot-reload (air)
	docker compose -f $(DEV_COMPOSE) up -d --build

dev-down: ## stop local environment
	docker compose -f $(DEV_COMPOSE) down

dev-logs: ## tail logs 
	docker compose -f $(DEV_COMPOSE) logs -f

dev-restart: ## rebuild and restart only the app
	docker compose -f $(DEV_COMPOSE) up -d --build --force-recreate app

# ---- migrations -----------------------------------------------------
# Ожидает переменную окружения DB_DSN, например:
#   export DB_DSN="host=localhost port=5432 user=postgres password=... dbname=astrobackend sslmode=disable"
migrate-up: ## apply all migrations
	goose -dir migrations postgres "$${DB_DSN}" up

migrate-down: ## rollback the last migration
	goose -dir migrations postgres "$${DB_DSN}" down

migrate-status: ## show migration status
	goose -dir migrations postgres "$${DB_DSN}" status

# ---- helpers --------------------------------------------------------
generate-key: ## generate ENCRYPTION_KEY (base64, 32 bytes) and write to $(ENV_FILE)
	@if [[ ! -f $(ENV_FILE) ]]; then touch $(ENV_FILE); fi; \
	if ! grep -q "^ENCRYPTION_KEY=" $(ENV_FILE); then \
		KEY=$$(head -c 32 /dev/urandom | base64 | tr -d '\n'); \
		printf "ENCRYPTION_KEY=%s\n" "$$KEY" >> $(ENV_FILE); \
		echo "ENCRYPTION_KEY added to $(ENV_FILE)"; \
	else \
		echo "ENCRYPTION_KEY already exists in $(ENV_FILE)"; \
	fi
