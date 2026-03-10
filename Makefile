.PHONY: build test lint clean install uninstall release publish

BINARY=tsm
MODULE=github.com/wake/tmux-session-menu

VERSION ?= $(shell cat VERSION 2>/dev/null || echo "dev")
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

PLATFORMS = darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

release:
	@mkdir -p bin
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output=bin/$(BINARY)-$$os-$$arch; \
		echo "Building $$output ..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -ldflags "$(LDFLAGS)" -o $$output ./cmd/tsm || exit 1; \
	done
	@echo "Done. Binaries in bin/"

REPO = wake/tmux-session-menu
ASSETS = $(foreach p,$(PLATFORMS),bin/$(BINARY)-$(subst /,-,$(p)))

publish: release
	@if gh release view v$(VERSION) -R $(REPO) >/dev/null 2>&1; then \
		echo "Error: release v$(VERSION) already exists"; exit 1; \
	fi
	git tag v$(VERSION)
	git push origin v$(VERSION)
	gh release create v$(VERSION) $(ASSETS) \
		-R $(REPO) \
		--title "v$(VERSION)" \
		--generate-notes

clean:
	rm -rf bin/ dist/ coverage.out

INSTALL_DIR ?= $(HOME)/.local/bin

install: build
	@mkdir -p $(INSTALL_DIR)
	install -m 755 bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)

uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)

run: build
	./bin/$(BINARY)
