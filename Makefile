VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY  := zerobased
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

LINK_TARGET := $(HOME)/.local/bin/$(BINARY)

.PHONY: build test clean cross npm install

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/zerobased

install: build
	@ln -sf $(CURDIR)/bin/$(BINARY) $(LINK_TARGET)
	@echo "$(LINK_TARGET) → $(CURDIR)/bin/$(BINARY)"

test:
	go test ./...

clean:
	rm -rf bin/ dist/

cross: clean
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		output=dist/$(BINARY)-$$os-$$arch; \
		echo "building $$platform → $$output"; \
		GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o $$output ./cmd/zerobased; \
	done

npm: cross
	@echo "copying binaries to npm packages..."
	@cp dist/$(BINARY)-linux-amd64 npm/linux-x64/$(BINARY)
	@cp dist/$(BINARY)-linux-arm64 npm/linux-arm64/$(BINARY)
	@cp dist/$(BINARY)-darwin-arm64 npm/darwin-arm64/$(BINARY)
	@cp dist/$(BINARY)-darwin-amd64 npm/darwin-x64/$(BINARY)
	@echo "done — run 'npm publish' in each npm/ subdirectory"
