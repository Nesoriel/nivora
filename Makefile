.PHONY: build test fmt vet run eval

build:
	go build -trimpath -o bin/nivora ./cmd/nivora
	go build -trimpath -o bin/nivora-eval ./cmd/nivora-eval

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...

run:
	go run ./cmd/nivora

eval:
	go run ./cmd/nivora-eval
