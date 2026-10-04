.PHONY: build run test clean install init

BINARY := picshare
# Inject the most recent git tag (without the leading "v") as the build
# version. Falls back to "dev" when no tags exist.
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

run: build
	./$(BINARY) --config config.json

test:
	go test -v ./...

clean:
	rm -f $(BINARY)
	rm -rf photos/* thumbs/*

init:
	./$(BINARY) --config config.json --init

install: build
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)

fmt:
	go fmt ./...

vet:
	go vet ./...

all: fmt vet test build
