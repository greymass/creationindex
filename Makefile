BINARY := creationindex
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

.PHONY: build install clean run tidy test vet verify

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/creationindex

install:
	@echo "==> Installing $(BINARY)"
	@go install $(LDFLAGS) ./cmd/creationindex

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	go clean

tidy:
	go mod tidy

test:
	go test ./...

vet:
	go vet ./...

verify: tidy vet build test
