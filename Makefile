BINARY := bin/zums-proxy-relay

.PHONY: build run test fmt vet lint

build:
	go build -trimpath -o $(BINARY) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -race ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	golangci-lint run
