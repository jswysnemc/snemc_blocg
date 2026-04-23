FRONTEND_DIR := frontend

.PHONY: frontend-install frontend-build build run test

frontend-install:
	cd $(FRONTEND_DIR) && npm install

frontend-build:
	cd $(FRONTEND_DIR) && npm run build

build: frontend-build
	go build -o snemc-blog ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...
