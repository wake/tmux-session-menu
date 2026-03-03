.PHONY: build test lint clean install uninstall

BINARY=tsm
MODULE=github.com/wake/tmux-session-menu

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -X $(MODULE)/internal/version.Version=$(VERSION) \
           -X $(MODULE)/internal/version.Commit=$(COMMIT) \
           -X $(MODULE)/internal/version.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/tsm

test:
	go test ./... -v -race

test-cover:
	go test ./... -coverprofile=coverage.out -race
	go tool cover -html=coverage.out

lint:
	go vet ./...

clean:
	rm -f bin/$(BINARY) coverage.out

install: build
	install -m 755 bin/$(BINARY) $$(go env GOPATH)/bin/$(BINARY)

uninstall:
	rm -f $$(go env GOPATH)/bin/$(BINARY)

run: build
	./bin/$(BINARY)
