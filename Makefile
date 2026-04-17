.PHONY: fmt lint test build run dev clean ci clean-memory gui gui-frontend gui-dev

BINARY_NAME := xbot

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

test:
	go test -v -race -coverprofile=coverage.out ./...

VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X xbot/version.Version=$(VERSION) -X xbot/version.Commit=$(shell git rev-parse --short HEAD) -X xbot/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GUI_LDFLAGS := $(LDFLAGS) -X xbot/version.BuildFlavor=gui

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME)

dev:
	go run -ldflags "$(LDFLAGS)" .

clean:
	rm -f $(BINARY_NAME) coverage.out
	go clean

ci: lint build test
	@echo "CI checks passed!"

clean-memory:
	rm -rf .xbot/
	@echo "Memory cleaned!"

gui-frontend:
	cd cmd/xbot-gui/frontend && npm install && npm run build

gui: gui-frontend
	rm -rf cmd/xbot-gui/build/bin/xbot-gui.app
	cd cmd/xbot-gui && wails build -tags gui
	@echo "Built xbot-gui.app (server embedded in single binary)"

gui-dev:
	cd cmd/xbot-gui && wails dev -tags gui
