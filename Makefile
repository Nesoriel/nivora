.PHONY: build test fmt vet run eval knowledge knowledge-eval

build:
	go build -trimpath -o bin/nivora ./cmd/nivora
	go build -trimpath -o bin/nivora-eval ./cmd/nivora-eval
	go build -trimpath -o bin/nivora-knowledge ./cmd/nivora-knowledge
	go build -trimpath -o bin/nivora-knowledge-eval ./cmd/nivora-knowledge-eval

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
