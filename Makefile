-include .env
-include .env.local
export

export GOOSE_DRIVER := postgres
export GOOSE_DBSTRING := $(DATABASE_URL)
export GOOSE_MIGRATION_DIR := migrations

.PHONY: run generate migrate migrate-down migrate-status test

run:
	go run ./cmd/trip-service

generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose up

migrate-down:
	go tool goose down

migrate-status:
	go tool goose status

test:
	go test -race ./...