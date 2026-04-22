FRONTEND_DIR := frontend

.PHONY: frontend-install frontend-build run test

frontend-install:
	cd $(FRONTEND_DIR) && npm install

frontend-build:
	cd $(FRONTEND_DIR) && npm run build

run:
	go run ./cmd/server

test:
	go test ./...
