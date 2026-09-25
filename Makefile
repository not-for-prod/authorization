# ==================================================================================== #
# PREPARE
# ==================================================================================== #

GOOSE_VERSION ?= v3.27.0
XO_VERSION ?= v0.0.1
GOIMPORTS_VERSION ?= v0.44.0
PROTO_FILES := $(shell find api -type f -name '*.proto' 2>/dev/null)

GOOSE ?= go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
XO ?= go run github.com/not-for-prod/xo@$(XO_VERSION)
TOOLS_BIN_DIR ?= /tmp/authorization-drivers-tools
GOIMPORTS_DIR = $(TOOLS_BIN_DIR)/goimports-$(GOIMPORTS_VERSION)
GOIMPORTS_BIN = $(GOIMPORTS_DIR)/goimports

$(GOIMPORTS_BIN):
	mkdir -p $(GOIMPORTS_DIR)
	GOBIN=$(GOIMPORTS_DIR) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

.PHONY: dependency
dependency:
	# mocks
	go install github.com/matryer/moq@latest
	# migrations
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	# reverse orm
	go install github.com/not-for-prod/xo@$(XO_VERSION)
	# xo formatter
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	# proto plugins
	go install github.com/not-for-prod/proterror/cmd/protoc-gen-proterror@latest
	go install github.com/not-for-prod/clay/cmd/protoc-gen-goclay@latest
	# code generators
	go install github.com/not-for-prod/implgen@latest
	# commonly used dependencies
	go get github.com/shopspring/decimal@latest
	go get github.com/not-for-prod/proterror@latest
	go get github.com/not-for-prod/clay@latest
	go get github.com/samber/lo@latest
	go get github.com/avito-tech/go-transaction-manager@latest
	go get github.com/google/uuid@latest


.PHONY: init
init: dependency
	./init.sh $(notdir $(CURDIR)) && \
	go mod tidy && \
	pre-commit install --config .pre-commit-config.yaml

# ==================================================================================== #
# INFRASTRUCTURE
# ==================================================================================== #

COMPOSE_PROJECT ?= authorization-drivers
COMPOSE_FILE ?= ./build/docker-compose.yaml
COMPOSE = docker compose --project-name $(COMPOSE_PROJECT) --file $(COMPOSE_FILE)

.PHONY: infra
infra:
	$(COMPOSE) up -d --wait postgres redis

.PHONY: infra-all
infra-all:
	$(COMPOSE) --profile full up -d --wait

.PHONY: infra-stop
infra-stop:
	$(COMPOSE) stop postgres redis

.PHONY: infra-down
infra-down:
	$(COMPOSE) down

# ==================================================================================== #
# MIGRATIONS
# ==================================================================================== #

DB_HOST ?= 127.0.0.1
DB_NAME ?= authorization
DB_PASS ?= postgres
DB_USER ?= postgres
DB_PORT ?= 55449
DB_DSN = host=$(DB_HOST) port=$(DB_PORT) user=$(DB_USER) password=$(DB_PASS) sslmode=disable dbname=$(DB_NAME)
DB_URL = postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
MIGRATION_FOLDER ?= ./tools/migrations

.PHONY: migrations-up ## накатывает миграции на базу данных
migrations-up:
	$(GOOSE) postgres '$(DB_DSN)' -dir $(MIGRATION_FOLDER) -allow-missing up

.PHONY: migrations-status
migrations-status:
	$(GOOSE) postgres '$(DB_DSN)' -dir $(MIGRATION_FOLDER) status

.PHONY: migrations-test
migrations-test:
	AUTHORIZATION_TEST_DATABASE_URL='$(DB_URL)' go test ./tools/migrations -count=1

.PHONY: migrations-reset ## накатывает миграции на базу данных
migrations-reset:
	@test "$(CONFIRM_DB_RESET)" = "$(DB_NAME)" || \
		(echo "Refusing to reset $(DB_NAME). Re-run with CONFIRM_DB_RESET=$(DB_NAME)."; exit 1)
	$(GOOSE) postgres '$(DB_DSN)' -dir $(MIGRATION_FOLDER) -allow-missing reset

.PHONY: migrations ## накатывает миграции на базу данных
migrations: migrations-reset migrations-up

# ==================================================================================== #
# CODEGEN
# ==================================================================================== #

XO_OUTPUT_PATH ?= ./generated/xo
XO_STAGING_PATH = $(XO_OUTPUT_PATH).tmp
XO_TEMPLATE_PATH ?= ./tools/xo-templates
.PHONY: xo ## генерация dto базы данных
xo: $(GOIMPORTS_BIN)
	rm -rf $(XO_STAGING_PATH)
	mkdir -p $(XO_STAGING_PATH)
	PATH="$(GOIMPORTS_DIR):$$PATH" $(XO) "$(DB_URL)" \
	-o $(XO_STAGING_PATH) --package xo --template-path $(XO_TEMPLATE_PATH) --schema public --suffix ".xo.go"
	rm -rf $(XO_OUTPUT_PATH)
	mv $(XO_STAGING_PATH) $(XO_OUTPUT_PATH)

.PHONY: pb
pb:
ifneq ($(strip $(PROTO_FILES)),)
	buf dep update
	buf dep prune
	buf lint
	#buf breaking --against ".git#subdir=."
	buf generate
else
	@echo "No protobuf schemas found; skipping protobuf generation."
endif

generate: pb xo
	go generate ./...

# ==================================================================================== #
# LINTER
# ==================================================================================== #

GOLANGCI_LINT ?= golangci-lint

linter:
	$(GOLANGCI_LINT) --config .golangci.yaml run

fmt:
	$(GOLANGCI_LINT) --config .golangci.yaml fmt

.PHONY: skills-validate
skills-validate:
	python3 .agents/scripts/validate_skills.py

.PHONY: drivers-test
drivers-test:
	AUTHORIZATION_TEST_DATABASE_URL='$(DB_URL)' AUTHORIZATION_TEST_REDIS_ADDR='127.0.0.1:$(or $(REDIS_PORT),56389)' go test -race ./driver/transactionmanager/pgx ./driver/session/redis ./internal/pkg/convert ./tools/migrations
