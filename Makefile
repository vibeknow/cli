BINARY := vibeknow
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/vibeknow/cli/cmd.version=$(VERSION)

.PHONY: build test lint install clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

install: build
	install -m 0755 $(BINARY) $(GOPATH)/bin/$(BINARY) || install -m 0755 $(BINARY) $(HOME)/go/bin/$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY)-* $(BINARY).exe
	rm -rf dist/
