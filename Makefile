VERSION ?= 1.1.0
export GOTOOLCHAIN ?= go1.27.1
.PHONY: test build security

test:
	go test -race ./...
	go vet ./...
	node --test tests/crypto.test.js

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/loggerbin .

security:
	go mod verify
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
