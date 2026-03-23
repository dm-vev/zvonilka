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

.PHONY: run-controlplane
run-controlplane:
	$(GO) run ./cmd/controlplane

.PHONY: run-gateway
run-gateway:
	$(GO) run ./cmd/gateway

.PHONY: run-botapi
run-botapi:
	$(GO) run ./cmd/botapi

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
