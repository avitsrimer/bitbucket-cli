BINARY      ?= bb
PKG         ?= ./cmd/bb
INSTALL_DIR ?= $(HOME)/bin

GO ?= GOTOOLCHAIN=local go

# build revision stamped into internal/cmd.version via -ldflags -X; "dev" when git is unavailable.
REV := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint fmt cross-build install

build:
	$(GO) build -ldflags "-X github.com/avitsrimer/bitbucket-cli/internal/cmd.version=$(REV)" -o $(BINARY) $(PKG)

test:
	$(GO) test -race ./...

lint:
	GOTOOLCHAIN=local golangci-lint run

fmt:
	gofmt -s -w .
	goimports -w .

# prove the repo stays cross-buildable even though releases are darwin/arm64 only.
cross-build:
	GOOS=linux CGO_ENABLED=0 $(GO) build ./...
	GOOS=linux CGO_ENABLED=0 $(GO) vet ./...

install: build
	install -d $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
