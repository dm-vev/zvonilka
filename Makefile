GO ?= go
CARGO ?= cargo
BUF ?= buf

.PHONY: fmt
fmt:
	$(GO) fmt ./...
	cd rust && $(CARGO) fmt --all

.PHONY: test
test:
	$(GO) test ./...
	cd rust && $(CARGO) test --workspace

.PHONY: build
build:
	$(GO) build ./...
	cd rust && $(CARGO) build --workspace

.PHONY: proto-lint
proto-lint:
	$(BUF) lint

.PHONY: proto-breaking
proto-breaking:
	$(BUF) breaking --against '.git#branch=main'

.PHONY: local-up
local-up:
	docker compose -f deploy/local/docker-compose.yml up -d

.PHONY: local-down
local-down:
	docker compose -f deploy/local/docker-compose.yml down -v
