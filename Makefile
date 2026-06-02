BINARY := k8s-installer-tui
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -s -w"

.PHONY: deps build build-linux clean

deps:
	go mod tidy

build: deps
	go build $(LDFLAGS) -o dist/$(BINARY) .

build-linux: deps
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 .

build-linux-arm64: deps
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 .

clean:
	rm -rf dist/
