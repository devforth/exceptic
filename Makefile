IMAGE ?= devforth/exceptic
VERSION ?= dev
MAX_IMAGE_MB ?= 50

.PHONY: test lint vulncheck fix build api-types web web-deps web-check dev run backup docker image-size release

test:
	go test -race ./...

# Needs golangci-lint v2 (config: .golangci.yml): https://golangci-lint.run/docs/welcome/install/
lint:
	golangci-lint run

# Known vulnerabilities of Go and the modules that the code actually calls (Go's own vulnerability database).
vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Fixes formatting, imports and blank lines; prints what is left to fix by hand.
fix:
	golangci-lint fmt
	golangci-lint run --fix

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/exceptic ./cmd/exceptic

# Rewrites what is generated from the code: the web app's types of the catalog (web/src/lib/api/catalog.gen.ts)
# and the parts of the docs the TestDocs* tests keep (tables, copied blocks, the values of the code they state);
# make test fails while they are stale. One package at a time: several write the same documents.
api-types:
	UPDATE_API_TYPES=1 go test -p 1 ./docs ./internal/... -run 'TestTypeScriptCatalog|TestDocs' -count=1

# The single-page app (web/, Node and pnpm): built into web/dist/spa, which Go embeds.
web: web-deps
	cd web && pnpm build

web-deps:
	cd web && pnpm install --frozen-lockfile

# Types (svelte-check), formatting (prettier), eslint and unit tests (vitest).
web-check: web-deps
	cd web && pnpm check && pnpm lint && pnpm test

# Development in one terminal, everything at http://localhost:8000 (PORT): the web app with hot
# reload, the Go server (built once, as `make run`) behind it on the next port. Ctrl+C stops both.
dev: web-deps
	@tools/dev.sh

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

# Publishes devforth/exceptic:VERSION for amd64 and arm64 from main once CI passed, and tags it (tools/release.sh).
release:
	tools/release.sh $(VERSION)
