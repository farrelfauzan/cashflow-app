API_DIR      := services/api
WEB_DIR      := apps/web
COMPOSE      := docker compose -f infra/compose.dev.yaml
SQLC         := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI_CODEGEN := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.1
GOLANGCI     := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK  := go run golang.org/x/vuln/cmd/govulncheck@v1.9.0
# Loads services/api/.env for the commands that need it.
WITH_ENV     := cd $(API_DIR) && set -a && . ./.env && set +a &&

.PHONY: help
help: ## List the targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

# ---- local environment ----------------------------------------------------------------

.PHONY: env
env: ## Create services/api/.env from the example if it is missing
	@test -f $(API_DIR)/.env || cp $(API_DIR)/.env.example $(API_DIR)/.env

.PHONY: dev-up
dev-up: ## Start PostgreSQL and Redis for local development
	$(COMPOSE) up -d --wait

.PHONY: dev-down
dev-down: ## Stop PostgreSQL, Redis and GOWA (data and the WhatsApp link are kept)
	$(COMPOSE) --profile whatsapp down

.PHONY: dev-reset
dev-reset: ## Stop everything and delete all data, including the GOWA WhatsApp link
	$(COMPOSE) --profile whatsapp down -v

.PHONY: wa-up
wa-up: ## Start the GOWA WhatsApp gateway; link a phone at http://localhost:3001
	$(COMPOSE) --profile whatsapp up -d gowa

.PHONY: wa-down
wa-down: ## Stop and remove GOWA (the linked session stays in its volume)
	$(COMPOSE) --profile whatsapp rm -sf gowa

# ---- run -------------------------------------------------------------------------------

.PHONY: migrate
migrate: env ## Apply database migrations as the owner role
	$(WITH_ENV) go run ./cmd/migrate

.PHONY: api-run
api-run: env ## Run the API
	$(WITH_ENV) go run ./cmd/api

.PHONY: worker-run
worker-run: env ## Run the worker
	$(WITH_ENV) go run ./cmd/worker

.PHONY: admin
admin: env ## Run the operator tool, e.g. make admin ARGS='create-org -name ... -member ...'
	$(WITH_ENV) go run ./cmd/admin $(ARGS)

# ---- build and check -------------------------------------------------------------------

.PHONY: api-build
api-build: ## Build the three binaries into services/api/bin
	cd $(API_DIR) && go build -trimpath -o bin/ ./cmd/...

.PHONY: api-docker
api-docker: ## Build the API image (api, worker and migrate in one image)
	docker build -t cashflow-api:dev $(API_DIR)

.PHONY: api-test
api-test: ## Unit tests with the race detector
	cd $(API_DIR) && go test -race ./...

.PHONY: api-test-integration
api-test-integration: ## Integration tests (needs Docker for testcontainers)
	cd $(API_DIR) && go test -race -tags integration ./...

.PHONY: api-lint
api-lint: ## go vet and golangci-lint
	cd $(API_DIR) && go vet ./... && $(GOLANGCI) run

.PHONY: api-vuln
api-vuln: ## Check dependencies for known vulnerabilities
	cd $(API_DIR) && $(GOVULNCHECK) ./...

.PHONY: sqlc
sqlc: ## Regenerate the database code from db/queries
	cd $(API_DIR) && $(SQLC) generate

.PHONY: sqlc-diff
sqlc-diff: ## Fail if the generated database code is stale
	cd $(API_DIR) && $(SQLC) diff

.PHONY: api-gen
api-gen: ## Regenerate the Go API types from api/openapi.yaml
	cd $(API_DIR) && $(OAPI_CODEGEN) -config oapi-codegen.yaml ../../api/openapi.yaml

.PHONY: api-check
api-check: api-lint sqlc-diff api-test api-test-integration api-vuln ## Everything CI runs

# ---- web (apps/web) --------------------------------------------------------------------

.PHONY: web-install
web-install: ## Install the web app's dependencies from the lockfile
	cd $(WEB_DIR) && pnpm install --frozen-lockfile

.PHONY: web-dev
web-dev: ## Run the web app on http://localhost:3000
	cd $(WEB_DIR) && pnpm dev

.PHONY: web-lint
web-lint: ## ESLint and the Prettier check for the web app
	cd $(WEB_DIR) && pnpm lint

.PHONY: web-typecheck
web-typecheck: ## Type-check the web app
	cd $(WEB_DIR) && pnpm typecheck

.PHONY: web-test
web-test: ## Web unit tests (Vitest)
	cd $(WEB_DIR) && pnpm test

.PHONY: web-e2e
web-e2e: ## Web end-to-end tests (Playwright, iPad and phone sizes)
	cd $(WEB_DIR) && pnpm e2e

.PHONY: web-e2e-dev
web-e2e-dev: ## Open every staff screen against `next dev`; fails on a Next.js dev error
	cd $(WEB_DIR) && E2E_SERVER=dev pnpm e2e

.PHONY: web-build
web-build: ## Production build of the web app
	cd $(WEB_DIR) && pnpm build

.PHONY: web-gen
web-gen: ## Regenerate the web API client (Orval) from api/openapi.yaml
	cd $(WEB_DIR) && pnpm gen

.PHONY: web-gen-check
web-gen-check: ## Fail if the generated web API client is stale
	cd $(WEB_DIR) && pnpm gen:check

.PHONY: web-check
web-check: web-gen-check web-lint web-typecheck web-test web-build ## Everything web CI runs except e2e
