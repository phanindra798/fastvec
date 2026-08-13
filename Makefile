GO ?= go

.PHONY: test race vet fmt build clean

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

build:
	mkdir -p bin
	$(GO) build -o bin/ ./cmd/...

clean:
	rm -rf bin
