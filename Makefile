APP_NAME ?= portal
IMAGE ?= satheeshds/$(APP_NAME)
PLATFORM_IMAGE ?= satheeshds/platform
TAG ?= latest
DOCKER ?= docker
COMPOSE ?= docker compose
ENV_FILE ?= .env

export DOCKER_BUILDKIT ?= 1

.PHONY: help image image-portal image-platform push compose-up compose-down build test

help: ## List available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "%-15s %s\n", $$1, $$2}'

image: image-portal image-platform ## Build Docker images for both portal and platform

image-portal: ## Build the portal Docker image
	$(DOCKER) build -t $(IMAGE):$(TAG) .

image-platform: ## Build the platform Docker image
	$(DOCKER) build -t $(PLATFORM_IMAGE):$(TAG) -f platform/Dockerfile .

push: ## Push the Docker image
	$(DOCKER) push $(IMAGE):$(TAG)

compose-up: ## Start the stack with docker compose
	$(COMPOSE) --env-file $(ENV_FILE) up -d --build

compose-down: ## Stop the stack
	$(COMPOSE) down

build: ## Build the Go binary
	go build .

test: ## Run Go tests
	go test ./...
