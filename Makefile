RAW_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
VERSION ?= $(if $(RAW_VERSION),$(patsubst v%,%,$(RAW_VERSION)),dev)
BIN := bin

.PHONY: all build app test race vet lint fmt check install clean

all: build

## build: build the CLI and the macOS window into bin/
build:
	@mkdir -p $(BIN)
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BIN)/exodrive ./cmd/exodrive
	go build -trimpath -o $(BIN)/exodrive-gui ./cmd/exodrive-gui

## app: bundle the window into bin/Exodrive.app and sign it
##      Set SIGN_IDENTITY to sign with a Developer ID; otherwise ad hoc.
app:
	@scripts/make-app.sh "$(VERSION)" "$(BIN)"

## notarize: submit bin/Exodrive.app to Apple, then staple the ticket
##      Needs a Developer ID build and NOTARY_PROFILE. See scripts/notarize.sh.
notarize:
	@scripts/notarize.sh "$(BIN)/Exodrive.app"

## test: run the test suite
test:
	go test -count=1 ./...

## race: run the test suite with the race detector
race:
	go test -race -count=1 ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format every Go file
fmt:
	gofmt -w .

## lint: fail on unformatted code, vet warnings, or lint warnings
lint:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "gofmt needed:"; echo "$$files"; exit 1; fi
	go vet ./...
	golangci-lint run ./...

## check: run the full gate (race tests and lint)
check: race lint

## install: install the CLI into GOBIN
install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/exodrive

clean:
	rm -rf $(BIN)

## help: list the targets
help:
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "%-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
