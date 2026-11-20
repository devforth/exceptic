IMAGE ?= devforth/exceptic
VERSION ?= dev
MAX_IMAGE_MB ?= 50

.PHONY: test lint fix build api-types web web-deps web-check web-dev run backup docker image-size

test:
	go test -race ./...

# Needs golangci-lint v2 (config: .golangci.yml): https://golangci-lint.run/docs/welcome/install/
lint:
	golangci-lint run

# Fixes formatting, imports and blank lines; prints what is left to fix by hand.
fix:
	golangci-lint fmt
	golangci-lint run --fix

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/exceptic ./cmd/exceptic

# Rewrites the web app's types of the catalog (web/src/lib/api/catalog.gen.ts); make test fails while they are stale.
api-types:
	UPDATE_API_TYPES=1 go test ./internal/api -run TestTypeScriptCatalog -count=1

# The single-page app (web/, Node and pnpm): built into web/dist/spa, which Go embeds.
web: web-deps
	cd web && pnpm build

web-deps:
	cd web && pnpm install --frozen-lockfile

# Types (svelte-check), formatting (prettier), eslint and unit tests (vitest).
web-check: web-deps
	cd web && pnpm check && pnpm lint && pnpm test

# The app with hot reload at http://localhost:5173: its pages here, every other address from `make run`.
web-dev: web-deps
	cd web && pnpm dev

# Loads .env and then .env.local (both optional, gitignored; see .env.example).
# Values from the files override the shell, so one-off variables like
# `LOG_LEVEL=debug make run` work only for keys the files don't set.
run:
	@set -a; [ -f .env ] && . ./.env; [ -f .env.local ] && . ./.env.local; set +a; \
	go run ./cmd/exceptic

# A backup through the running server (or of the stopped database): DIR=/abs/path, by default DATA_DIR/backups/<time>.
backup:
	@set -a; [ -f .env ] && . ./.env; [ -f .env.local ] && . ./.env.local; set +a; \
	go run ./cmd/exceptic backup $(DIR)

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

# Fails if the image is larger than MAX_IMAGE_MB.
image-size:
	@size=$$(docker image inspect $(IMAGE):$(VERSION) --format '{{.Size}}'); \
	mb=$$((size / 1024 / 1024)); \
	echo "image $(IMAGE):$(VERSION): $${mb} MB (limit $(MAX_IMAGE_MB) MB)"; \
	test $$mb -le $(MAX_IMAGE_MB)
