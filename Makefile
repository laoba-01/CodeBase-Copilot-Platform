.PHONY: dev build run test

dev:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./internal/... -v -count=1

db-up:
	docker compose up -d db redis

dc-up:
	docker compose up -d --build

dc-down:
	docker compose down

proto:
	protoc --go_out=. --go-grpc_out=. proto/embedding.proto
	python -m grpc_tools.protoc -I proto --python_out=python-embedding --grpc_python_out=python-embedding proto/embedding.proto