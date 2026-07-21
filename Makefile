.PHONY: all build server mcp cli deps migrate dev clean docker-up docker-down release

# ── Config ───────────────────────────────────────────────────────────────────

GOFLAGS := -ldflags="-s -w"
BINDIR  := ./bin

# ── Build ─────────────────────────────────────────────────────────────────────

all: deps build

build: server mcp cli

server:
	go build $(GOFLAGS) -o $(BINDIR)/hiveshare-server ./cmd/server

mcp:
	go build $(GOFLAGS) -o $(BINDIR)/hiveshare-mcp ./cmd/mcp

cli:
	go build $(GOFLAGS) -o $(BINDIR)/hshare ./cmd/hshare

deps:
	go mod tidy
	go mod download

# ── Database ─────────────────────────────────────────────────────────────────

POSTGRES_URL ?= postgres://hiveshare:hiveshare@localhost:5432/hiveshare?sslmode=disable

migrate:
	@echo "Applying migrations..."
	@for f in migrations/*.sql; do \
	    echo "  Running $$f..."; \
	    psql "$(POSTGRES_URL)" -f "$$f"; \
	done
	@echo "Migrations done."

# ── Dev ───────────────────────────────────────────────────────────────────────

dev: docker-up
	@echo "Waiting for postgres..."
	@sleep 3
	@$(MAKE) migrate
	@echo "Starting server..."
	EMBED_PROVIDER= go run ./cmd/server

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# ── Install CLI ───────────────────────────────────────────────────────────────

install: cli
	@cp $(BINDIR)/hshare /usr/local/bin/hshare
	@echo "hshare installed to /usr/local/bin/hshare"

install-mcp: mcp
	@cp $(BINDIR)/hiveshare-mcp /usr/local/bin/hiveshare-mcp
	@echo "hiveshare-mcp installed to /usr/local/bin/hiveshare-mcp"

# ── Release (local cross-compile) ─────────────────────────────────────────────

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
RELEASE_LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

release:
	@echo "Building release binaries for version $(VERSION)..."
	@mkdir -p dist
	@for GOOS in linux darwin; do \
	  for GOARCH in amd64 arm64; do \
	    PLATFORM=$${GOOS}_$${GOARCH}; \
	    OUT=dist/$$PLATFORM; \
	    mkdir -p $$OUT; \
	    echo "  Building $$PLATFORM..."; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hiveshare-server ./cmd/server; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hiveshare-mcp    ./cmd/mcp; \
	    GOOS=$$GOOS GOARCH=$$GOARCH go build $(RELEASE_LDFLAGS) -o $$OUT/hshare           ./cmd/hshare; \
	    tar -czf dist/hiveshare_$${PLATFORM}.tar.gz -C $$OUT .; \
	    echo "  → dist/hiveshare_$${PLATFORM}.tar.gz"; \
	  done; \
	done
	@echo "Done. Tarballs in dist/"

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BINDIR) dist/

# ── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo "HiveShare Makefile targets:"
	@echo ""
	@echo "  make dev          Start docker, run migrations, start server"
	@echo "  make build        Build all binaries to ./bin/"
	@echo "  make server       Build API server"
	@echo "  make mcp          Build MCP sidecar"
	@echo "  make cli          Build hshare CLI"
	@echo "  make migrate      Apply SQL migrations"
	@echo "  make install      Install hshare to /usr/local/bin"
	@echo "  make install-mcp  Install hiveshare-mcp to /usr/local/bin"
	@echo "  make docker-up    Start postgres + redis"
	@echo "  make docker-down  Stop postgres + redis"
	@echo "  make release      Cross-compile all platforms → dist/"
	@echo "  make clean        Remove ./bin/ and dist/"
