.PHONY: build test lint demo
build:  ; go build -o bin/node ./cmd/node
test:   ; go test ./... -race -count=1
lint:   ; go vet ./...
fmt:
	goimports -w $(shell find . -name '*.go' -not -path './vendor/*')