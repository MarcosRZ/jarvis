CLIENT_DIR := client
BIN_DIR := $(CLIENT_DIR)/bin
BINARY := voice-client
VERSION ?= dev

SERVER_DIR := server
HA_DIR := $(SERVER_DIR)/homeassistant
HA_URL := http://127.0.0.1:8123
HA_TOKEN_FILE := $(CURDIR)/$(HA_DIR)/dev-token
HA_PASSWORD_FILE := $(CURDIR)/$(HA_DIR)/dev-password
HA_COMPOSE := docker compose --profile homeassistant

# make run reads the client settings from client/.env.$(ENV): dev or prod.
ENV := dev
ENV_FILE := $(CLIENT_DIR)/.env.$(ENV)

.PHONY: build test integration lint clean services-up services-down services-status services-logs ha-up ha-down ha-reset run windows

build:
	cd $(CLIENT_DIR) && go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/voice-client

# Builds client/bin/voice-client.exe for Windows amd64 in a container with
# MinGW (cgo for the audio library). It needs only Windows system DLLs.
WINDOWS_BUILDER := golang:1.27.2-bookworm
windows:
	docker run --rm -v "$(CURDIR)/$(CLIENT_DIR)":/src -v jarvis-gomod:/go/pkg/mod -w /src $(WINDOWS_BUILDER) sh -c '\
		apt-get update -qq >/dev/null && apt-get install -y -qq gcc-mingw-w64-x86-64 >/dev/null && \
		CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
		go build -ldflags "-X main.version=$(VERSION) -extldflags=-static" -o bin/$(BINARY).exe ./cmd/voice-client'
	@ls -l $(BIN_DIR)/$(BINARY).exe

test:
	cd $(CLIENT_DIR) && go test ./...

# The Home Assistant tests run when the development Home Assistant is set up (make ha-up).
integration:
	@if [ -f $(HA_TOKEN_FILE) ]; then export VOICE_HA_URL=$(HA_URL) VOICE_HA_TOKEN="$$(cat $(HA_TOKEN_FILE))"; fi; \
	cd $(CLIENT_DIR) && go test -tags integration -count=1 -run Integration ./...

lint:
	@unformatted="$$(cd $(CLIENT_DIR) && gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	cd $(CLIENT_DIR) && go vet ./...

clean:
	rm -rf $(BIN_DIR)

services-up:
	cd $(SERVER_DIR) && docker compose up -d --wait

services-down:
	cd $(SERVER_DIR) && $(HA_COMPOSE) down

services-status:
	scripts/status.sh

services-logs:
	cd $(SERVER_DIR) && docker compose logs --tail 100 --follow

ha-up:
	cd $(SERVER_DIR) && $(HA_COMPOSE) up -d --wait homeassistant
	cd $(CLIENT_DIR) && go run ./cmd/ha-dev-setup --url $(HA_URL) --token-file $(HA_TOKEN_FILE) --password-file $(HA_PASSWORD_FILE)

ha-down:
	cd $(SERVER_DIR) && $(HA_COMPOSE) rm --stop --force homeassistant

# Deletes all Home Assistant data except configuration.yaml.
ha-reset: ha-down
	find $(HA_DIR) -mindepth 1 ! -name configuration.yaml -exec rm -rf {} +

# The settings come only from the env file: VOICE_* variables of the shell are
# removed first, so a dev run never picks up a prod token. ARGS adds flags.
run: build
	@test -f $(ENV_FILE) || { echo "$(ENV_FILE) is missing; copy it from $(ENV_FILE).example"; exit 1; }
	@unset $$(env | sed -n 's/^\(VOICE_[A-Z_]*\)=.*/\1/p'); \
	set -a; . ./$(ENV_FILE); set +a; \
	if [ "$(ENV)" = dev ] && [ -z "$${VOICE_HA_TOKEN:-}" ] && [ -f $(HA_TOKEN_FILE) ]; then \
		VOICE_HA_TOKEN="$$(cat $(HA_TOKEN_FILE))"; export VOICE_HA_TOKEN; \
	fi; \
	exec $(BIN_DIR)/$(BINARY) run $(ARGS)
