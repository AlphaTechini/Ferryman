GO      ?= go
BIN     := bin/ferryman
PKG     := ./cmd/ferryman
IMAGE   ?= ferryman-keeper

.PHONY: all build test vet fmt run docker clean

all: fmt vet test build

build:
	$(GO) build -trimpath -o $(BIN) $(PKG)

test:
	$(GO) test -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -s -w .

# Runs the keeper with the current environment. Set KEEPER_PRIVATE_KEY and DRY_RUN as needed (see .env.example).
run: build
	./$(BIN)

docker:
	docker build -t $(IMAGE) .

clean:
	rm -rf bin
