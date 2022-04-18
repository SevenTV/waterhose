all: build_deps proto linux

BUILDER := "unknown"
VERSION := "unknown"

ifeq ($(origin WATERHOSE_BUILDER),undefined)
	BUILDER = $(shell git config --get user.name);
else
	BUILDER = ${WATERHOSE_BUILDER};
endif

ifeq ($(origin WATERHOSE_VERSION),undefined)
	VERSION = $(shell git rev-parse HEAD);
else
	VERSION = ${WATERHOSE_VERSION};
endif

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags "-X 'main.Version=${VERSION}' -X 'main.Unix=$(shell date +%s)' -X 'main.User=${BUILDER}'" -o bin/waterhose .
	
lint:
	staticcheck ./...
	go vet ./...
	golangci-lint run
	yarn prettier --write .

	$(MAKE) -C protobuf lint

deps: build_deps
	go mod download
	yarn
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

	$(MAKE) -C protobuf deps

build_deps:
	$(MAKE) -C protobuf build_deps

proto:
	$(MAKE) -C protobuf compile

test:
	go test -count=1 -cover ./...
