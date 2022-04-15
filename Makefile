all: proto loader linux

BUILDER := "unknown"
VERSION := "unknown"

ifeq ($(origin TWITCH_CHAT_CONTROLLER_BUILDER),undefined)
	BUILDER = $(shell git config --get user.name);
else
	BUILDER = ${TWITCH_CHAT_CONTROLLER_BUILDER};
endif

ifeq ($(origin TWITCH_CHAT_CONTROLLER_VERSION),undefined)
	VERSION = $(shell git rev-parse HEAD);
else
	VERSION = ${TWITCH_CHAT_CONTROLLER_VERSION};
endif

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags "-X 'main.Version=${VERSION}' -X 'main.Unix=$(shell date +%s)' -X 'main.User=${BUILDER}'" -o bin/twitch-edge .
	
lint:
	staticcheck ./...
	go vet ./...
	golangci-lint run
	yarn prettier --write .

	$(MAKE) -C protobuf lint

deps:
	go mod download
	yarn
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

	$(MAKE) -C protobuf deps
	$(MAKE) -C loaders deps

proto:
	$(MAKE) -C protobuf compile

loader:
	$(MAKE) -C loaders compile

test:
	go test -count=1 -cover ./...
