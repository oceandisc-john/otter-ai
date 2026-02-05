# Otter-AI Monorepo Makefile
# The only supported interface for build and runtime control

.PHONY: help all otter kelpie up down purge

# Default target
help:
	@echo "Otter-AI Monorepo Control"
	@echo ""
	@echo "Usage: make <service|all> <action>"
	@echo ""
	@echo "Services:"
	@echo "  otter      - Otter-AI backend"
	@echo "  kelpie     - Kelpie UI frontend"
	@echo "  all        - All services"
	@echo ""
	@echo "Actions:"
	@echo "  up         - Start service(s)"
	@echo "  down       - Stop service(s)"
	@echo "  purge      - Remove service(s) and data"
	@echo ""
	@echo "Examples:"
	@echo "  make all up"
	@echo "  make otter up"
	@echo "  make kelpie down"
	@echo "  make all purge"

# Service: All
all: ## Target all services
	@:

all-up:
	docker compose up -d --build

all-down:
	docker compose down

all-purge:
	docker compose down -v --remove-orphans
	docker system prune -f

# Service: Otter-AI Backend
otter: ## Target otter-ai backend
	@:

otter-up:
	docker compose up -d --build otter-ai

otter-down:
	docker compose stop otter-ai

otter-purge:
	docker compose down otter-ai -v
	docker rmi otter-ai:latest 2>/dev/null || true

# Service: Kelpie UI Frontend
kelpie: ## Target kelpie-ui frontend
	@:

kelpie-up:
	docker compose up -d --build kelpie-ui

kelpie-down:
	docker compose stop kelpie-ui

kelpie-purge:
	docker compose down kelpie-ui -v
	docker rmi kelpie-ui:latest 2>/dev/null || true

# Action routing
up: all-up
down: all-down
purge: all-purge
