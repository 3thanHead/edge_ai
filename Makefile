GO ?= go
SERVICES := gateway ai-service simulator
DIST := dist
COMPOSE := docker compose -f deploy/docker-compose.yml

.PHONY: help tidy fmt vet test build clean docker-up docker-sim docker-down

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

tidy: ## Sync go.mod / go.sum
	$(GO) mod tidy

fmt: ## Format all Go code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

test: ## Run tests with the race detector
	$(GO) test -race ./...

build: ## Build all service binaries into ./dist
	@mkdir -p $(DIST)
	@for s in $(SERVICES); do \
		echo "building $$s"; \
		$(GO) build -trimpath -o $(DIST)/$$s ./cmd/$$s; \
	done

run-%: ## Run one service locally, e.g. make run-gateway
	$(GO) run ./cmd/$*

docker-up: ## Build & start broker + gateway + ai-service
	$(COMPOSE) up --build

docker-sim: ## Build & start the full stack including the simulator
	$(COMPOSE) --profile sim up --build

docker-down: ## Stop the stack and remove volumes
	$(COMPOSE) down -v

clean: ## Remove build artifacts
	rm -rf $(DIST)
