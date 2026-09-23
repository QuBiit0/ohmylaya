.PHONY: build test lint vet tidy clean

BIN := bin/ohmylaya
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/QuBiit0/ohmylaya/internal/buildinfo.Version=$(VERSION) \
	-X github.com/QuBiit0/ohmylaya/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/QuBiit0/ohmylaya/internal/buildinfo.Date=$(DATE)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ohmylaya

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

lint: vet
	go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...

tidy:
	go mod tidy

clean:
	rm -rf bin dist
