.PHONY: build lint deps dev_deps

BUILDER := "unknown"
VERSION := "unknown"

ifeq ($(origin WATERHOSE_BUILDER),undefined)
	BUILDER = $(shell git config --get user.name)
else
	BUILDER = ${WATERHOSE_BUILDER}
endif

ifeq ($(origin WATERHOSE_VERSION),undefined)
	VERSION = $(shell git rev-parse HEAD)
else
	VERSION = ${WATERHOSE_VERSION}
endif

build:
	$(MAKE) -C protobuf compile
	GOOS=linux GOARCH=amd64 go build -v -ldflags "-X 'main.Version=${VERSION}' -X 'main.Unix=$(shell date +%s)' -X 'main.User=${BUILDER}'" -o out/waterhose cmd/*.go

lint:
	yarn prettier --check .
	staticcheck ./...
	go vet ./...
	golangci-lint run
	
	$(MAKE) -C protobuf lint

format:
	yarn prettier --write .
	gofmt -s -w .

deps:
	go mod download
	$(MAKE) -C protobuf build_deps

dev_deps:
	go install honnef.co/go/tools/cmd/staticcheck@v0.3.1
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@master # TODO fix this version, currently master to lint go1.18 files

	$(MAKE) -C protobuf deps

test:
	$(MAKE) -C protobuf compile
	go test -count=1 -cover -parallel $$(nproc) -race ./...

clean:
	rm -rf bin
