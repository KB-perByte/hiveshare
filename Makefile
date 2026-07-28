.PHONY: all build server mcp cli deps migrate dev dev-clean clean docker-up docker-down release server-linux \
       smoke-test smoke-test-full integration-test psql install-mcp update-mcp

# ── Config ───────────────────────────────────────────────────────────────────

BINDIR  := ./bin
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILDTIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_LDFLAGS := -X github.com/KB-perByte/hiveshare/internal/version.Commit=$(COMMIT) \
	-X github.com/KB-perByte/hiveshare/internal/version.BuildTime=$(BUILDTIME)
GOFLAGS := -ldflags="-s -w $(VERSION_LDFLAGS)"
CONTAINER_RUNTIME ?= docker

# ── Build ─────────────────────────────────────────────────────────────────────

all: deps build

# Install CLI + MCP sidecar and wire into Claude Code / Cursor.
install-mcp:
	./scripts/install-client.sh

# Quick MCP binary rebuild — no prompts, keeps existing credentials.
update-mcp: mcp
	@BINARY="$${HOME}/.local/bin/hiveshare-mcp"; \
	rm -f "$$BINARY"; \
	install -m 755 $(BINDIR)/hiveshare-mcp "$$BINARY"; \
	echo "  Updated $$BINARY"

build: server mcp cli

server:
	go build $(GOFLAGS) -o $(BINDIR)/hiveshare-server ./cmd/server

# Cross-compile for EC2 (Ubuntu amd64)
server-linux:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(BINDIR)/hiveshare-server-linux ./cmd/server
	@echo "Built $(BINDIR)/hiveshare-server-linux commit=$(COMMIT) build_time=$(BUILDTIME)"

mcp:
	go build $(GOFLAGS) -o $(BINDIR)/hiveshare-mcp ./cmd/mcp

cli:
	go build $(GOFLAGS) -o $(BINDIR)/hiveshare ./cmd/hiveshare

deps:
	go mod tidy
	go mod download

# ── Database ─────────────────────────────────────────────────────────────────

POSTGRES_URL ?= postgres://hiveshare:hiveshare@localhost:5432/hiveshare?sslmode=disable
# Compose service name (stable across project prefixes / renamed containers).
POSTGRES_SERVICE ?= postgres

migrate:
	@echo "Applying migrations..."
	@if command -v psql >/dev/null 2>&1; then \
	    for f in migrations/*.sql; do \
	        echo "  Running $$f..."; \
	        psql "$(POSTGRES_URL)" -f "$$f"; \
	    done; \
	elif $(CONTAINER_RUNTIME) compose ps --status running $(POSTGRES_SERVICE) >/dev/null 2>&1; then \
	    for f in migrations/*.sql; do \
	        echo "  Running $$f (via compose $(POSTGRES_SERVICE))..."; \
	        $(CONTAINER_RUNTIME) compose exec -T $(POSTGRES_SERVICE) \
	            psql -U hiveshare -d hiveshare -f - < "$$f"; \
	    done; \
	else \
	    echo "Error: psql not found and compose service '$(POSTGRES_SERVICE)' is not running"; \
	    exit 1; \
	fi
	@echo "Migrations done."

# ── Dev ───────────────────────────────────────────────────────────────────────

dev: docker-up
	@echo "Waiting for postgres..."
	@sleep 3
	@$(MAKE) migrate
	@echo "Starting server..."
	EMBED_PROVIDER= go run ./cmd/server

docker-up:
	$(CONTAINER_RUNTIME) compose up -d

docker-down:
	$(CONTAINER_RUNTIME) compose down

dev-clean:
	$(CONTAINER_RUNTIME) compose down -v

psql:
	@echo "**INFO**: Opening psql via '$(CONTAINER_RUNTIME) compose exec $(POSTGRES_SERVICE)'"
	$(CONTAINER_RUNTIME) compose exec -it $(POSTGRES_SERVICE) psql -U hiveshare -d hiveshare
	
# ── Install CLI ───────────────────────────────────────────────────────────────

install: cli
	@cp $(BINDIR)/hiveshare /usr/local/bin/hiveshare
	@echo "hiveshare installed to /usr/local/bin/hiveshare"

install-mcp-system: mcp
	@cp $(BINDIR)/hiveshare-mcp /usr/local/bin/hiveshare-mcp
	@echo "hiveshare-mcp installed to /usr/local/bin/hiveshare-mcp"

# ── Release (local cross-compile) ─────────────────────────────────────────────

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
RELEASE_LDFLAGS := -ldflags="-s -w $(VERSION_LDFLAGS) -X main.version=$(VERSION)"

release:
	@echo "Building release binaries for version $(VERSION) commit=$(COMMIT)..."
	@mkdir -p dist
	@for GOOS in linux darwin; do \
	  for GOARCH in amd64 arm64; do \
	    PLATFORM=$${GOOS}_$${GOARCH}; \
	    OUT=dist/$$PLATFORM; \
	    mkdir -p $$OUT; \
	    echo "  Building $$PLATFORM..."; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hiveshare-server ./cmd/server; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hiveshare-mcp    ./cmd/mcp; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hiveshare       ./cmd/hiveshare; \
	    tar -czf dist/hiveshare_$${PLATFORM}.tar.gz -C $$OUT .; \
	    echo "  → dist/hiveshare_$${PLATFORM}.tar.gz"; \
	  done; \
	done
	@echo "Done. Tarballs in dist/"

# ── Test ──────────────────────────────────────────────────────────────────────

HIVESHARE_TEST_URL ?= $(or $(BASE_URL),http://localhost:8080)

smoke-test:
	@./scripts/smoke-test.sh $(HIVESHARE_TEST_URL)

smoke-test-full:
	@./scripts/smoke-test-full.sh $(HIVESHARE_TEST_URL)

integration-test:
	HIVESHARE_TEST_URL=$(HIVESHARE_TEST_URL) pytest tests/ -v

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BINDIR) dist/

# ── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo "HiveShare Makefile targets:"
	@echo ""
	@echo "  make dev            Start docker, run migrations, start server"
	@echo "  make build          Build all binaries to ./bin/"
	@echo "  make server         Build API server (embeds git commit + build time)"
	@echo "  make server-linux   Cross-compile API for EC2 Ubuntu amd64"
	@echo "  make mcp            Build MCP sidecar"
	@echo "  make cli            Build hiveshare CLI"
	@echo "  make migrate        Apply SQL migrations"
	@echo "  make install            Install hiveshare CLI to /usr/local/bin (root)"
	@echo "  make install-mcp        Interactive guided install: CLI + MCP to ~/.local/bin"
	@echo "  make install-mcp-system Install hiveshare-mcp to /usr/local/bin (root)"
	@echo "  make update-mcp         Rebuild MCP binary and update ~/.local/bin"
	@echo "  make docker-up      Start postgres + redis"
	@echo "  make docker-down    Stop postgres + redis"
	@echo "  make dev-clean      Stop containers and wipe database volume"
	@echo "  make psql           Open psql shell in the running postgres container"
	@echo "  make smoke-test     Basic connectivity check (no user needed)"
	@echo "  make smoke-test-full  Full endpoint smoke test (curl + jq)"
	@echo "  make integration-test  Pytest integration tests"
	@echo "  make release        Cross-compile all platforms → dist/"
	@echo "  make clean          Remove ./bin/ and dist/"
