ENV_FILE ?= ../../infra/.env
ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: run migrate build tidy test

run:
	go run ./cmd/api

migrate:
	go run ./cmd/api migrate

build:
	go build -o bin/api ./cmd/api

tidy:
	go mod tidy

test:
	go test ./...
