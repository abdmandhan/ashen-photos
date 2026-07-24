ENV_FILE ?= ../../infra/.env
ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: run build tidy test

run:
	go run ./cmd/worker

build:
	go build -o bin/worker ./cmd/worker

tidy:
	go mod tidy

test:
	go test ./...
