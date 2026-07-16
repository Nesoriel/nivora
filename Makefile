.PHONY: build test fmt vet run eval knowledge knowledge-eval probe load shadow test-provider

build:
	go build -trimpath -o bin/nivora ./cmd/nivora
	go build -trimpath -o bin/nivora-eval ./cmd/nivora-eval
	go build -trimpath -o bin/nivora-knowledge ./cmd/nivora-knowledge
	go build -trimpath -o bin/nivora-knowledge-eval ./cmd/nivora-knowledge-eval
	go build -trimpath -o bin/nivora-probe ./cmd/nivora-probe
	go build -trimpath -o bin/nivora-load ./cmd/nivora-load
	go build -trimpath -o bin/nivora-shadow ./cmd/nivora-shadow
	go build -trimpath -o bin/nivora-test-provider ./cmd/nivora-test-provider

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

knowledge:
	go run ./cmd/nivora-knowledge

knowledge-eval:
	go run ./cmd/nivora-knowledge-eval

probe:
	go run ./cmd/nivora-probe

load:
	go run ./cmd/nivora-load

shadow:
	go run ./cmd/nivora-shadow

test-provider:
	go run ./cmd/nivora-test-provider
