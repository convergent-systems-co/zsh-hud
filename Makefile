.PHONY: build test lint vet

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	gofmt -l .
