.PHONY: build run test test-unit lint lint-fix migrate-up migrate-down docker-build docker-up docker-down swagger seed-hsn generate-hsn-seed backfill-summaries backfill-normalize-tags

include .env
export $(shell sed 's/=.*//' .env)

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./... -v -count=1

test-race:
	go test -race ./... -v -count=1

test-unit:
	go test ./tests/unit/... -v -count=1

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

docker-build:
	docker build -t satvos:latest .

docker-up:
	docker compose up -d

docker-down:
	docker compose down -v

swagger:
	swag init -g cmd/server/main.go -o docs

generate-hsn-seed:
	go run ./cmd/seedhsn

seed-hsn:
	psql "postgres://$(SATVOS_DB_USER):$(SATVOS_DB_PASSWORD)@$(SATVOS_DB_HOST):$(SATVOS_DB_PORT)/$(SATVOS_DB_NAME)?sslmode=$(SATVOS_DB_SSLMODE)" -f db/seeds/hsn_codes.sql

backfill-summaries:
	go run ./cmd/backfill

backfill-normalize-tags:
	go run ./cmd/backfill-normalize-tags