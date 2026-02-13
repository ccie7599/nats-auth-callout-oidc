.PHONY: help build up down demo demo-ping dashboard logs logs-auth logs-nats test certs certs-check clean

DEMO_DOMAIN ?= nats-demo.connected-cloud.io
COMPOSE := docker compose

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all container images
	$(COMPOSE) --profile cli --profile dashboard build

up: ## Start core services (NATS, auth)
	$(COMPOSE) up -d
	@echo ""
	@echo "NATS:      tls://localhost:4222"
	@echo "NATS Mon:  http://localhost:8222"

up-all: ## Start all services including dashboard
	$(COMPOSE) --profile dashboard up -d
	@echo ""
	@echo "Dashboard: https://$(DEMO_DOMAIN)"
	@echo "NATS WSS:  wss://$(DEMO_DOMAIN):8443"

down: ## Stop all services and remove volumes
	$(COMPOSE) --profile cli --profile dashboard down -v

demo: ## Run all 6 CLI demo scenarios
	$(COMPOSE) --profile cli run --rm demo-client --scenario all

demo-admin: ## Run admin scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario admin

demo-publisher: ## Run publisher scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario publisher

demo-subscriber: ## Run subscriber scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario subscriber

demo-invalid: ## Run invalid token scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario invalid

demo-notoken: ## Run no-token scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario notoken

demo-ping: ## Run PingOne live scenario only
	$(COMPOSE) --profile cli run --rm demo-client --scenario ping

test: ## Run unit tests
	cd auth-service && go test -v ./...

logs: ## Tail all service logs
	$(COMPOSE) logs -f

logs-auth: ## Tail auth-service logs
	$(COMPOSE) logs -f auth-service

logs-nats: ## Tail NATS server logs
	$(COMPOSE) logs -f nats

certs: ## Generate TLS certs via Let's Encrypt + Akamai DNS-01
	DEMO_DOMAIN=$(DEMO_DOMAIN) sudo -E bash scripts/setup-certs.sh

certs-check: ## Check cert validity
	@openssl x509 -in certs/fullchain.pem -noout -dates -subject 2>/dev/null || \
		echo "No certs found. Run: make certs"

clean: ## Full cleanup
	$(COMPOSE) --profile cli --profile dashboard down -v --rmi local 2>/dev/null || true
	rm -rf certs/*.pem
