GO ?= go
NPM ?= npm

.PHONY: build test vet lint vulncheck ui-build pricing-sync check clean

build:
	$(GO) build -o bin/tok ./cmd/tok

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

vulncheck:
	govulncheck ./...

ui-build:
	$(NPM) --prefix ui install
	$(NPM) --prefix ui run build

pricing-sync:
	cp pricing/catalog.json internal/pricing/embedded_catalog.json

check: test vet lint

clean:
	rm -rf bin dist ui/node_modules
