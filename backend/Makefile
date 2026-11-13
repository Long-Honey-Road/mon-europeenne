GO ?= go
TOOL := $(GO) tool -modfile=tools.mod
COVER_OUT ?= coverage.out
CMD ?= api

# Generated code and mock services are excluded from coverage, as the case allows.
COVER_PKGS = $(shell $(GO) list ./... | grep -v -e '/gen/' -e '/internal/mocks' | paste -sd, -)

.PHONY: build test cover lint lint-go lint-proto fmt gen gen-check run tidy

build:
	$(GO) build -o bin/buddy ./cmd/buddy

test:
	$(GO) test -race ./...

cover:
	$(GO) test -race -covermode=atomic -coverpkg=$(COVER_PKGS) -coverprofile=$(COVER_OUT) ./...
	$(GO) tool cover -func=$(COVER_OUT) | tail -n 1

lint: lint-go lint-proto

lint-go:
	$(TOOL) golangci-lint run ./...

lint-proto:
	$(TOOL) buf lint
	$(TOOL) buf format --diff --exit-code api/proto

fmt:
	$(TOOL) golangci-lint fmt ./...
	$(TOOL) buf format --write api/proto

gen:
	$(TOOL) buf generate
	$(TOOL) buf generate --template buf.gen.api.yaml
	mv api/openapi/openapi.yaml api/openapi/buddy-api.yaml
	$(TOOL) buf generate --template buf.gen.mocks.yaml
	mv api/openapi/openapi.yaml api/openapi/mocks-api.yaml

# Fails if committed generated code is stale.
gen-check: gen
	git diff --exit-code -- gen api/openapi
	@test -z "$$(git status --porcelain -- gen api/openapi)" || (git status --porcelain -- gen api/openapi; exit 1)

# Usage: make run CMD=mocks
run:
	$(GO) run ./cmd/buddy $(CMD)

tidy:
	$(GO) mod tidy
	$(GO) mod tidy -modfile=tools.mod
