.PHONY: deps dev test test-integration lint

deps:
	go mod download

dev:
	docker compose -f deploy/local.yml up -d postgres kafka
	go run ./cmd/messenger-api -config config/local.yaml

test:
	go test -race ./...

test-integration:
	go test -race -tags integration ./...

lint:
	golangci-lint run ./...
