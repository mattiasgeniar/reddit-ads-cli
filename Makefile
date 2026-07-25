BINARY := reddit-ads
PKG := ./cmd/reddit-ads
INSTALL_DIR := $(HOME)/bin

.PHONY: build test install fmt vet clean

build:
	go build -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w internal/ cmd/

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist/
