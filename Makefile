.PHONY: build test fmt vet run

build:
	go build -trimpath -o bin/nivora ./cmd/nivora

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...

run:
	go run ./cmd/nivora
