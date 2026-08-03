.PHONY: build run test clean install init

BINARY := picshare
VERSION := 1.0.0
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
