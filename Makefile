VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tawhidjoarder/adastra/cmd.Version=$(VERSION)

.PHONY: build test lint install clean release-snapshot

build:
	go build -ldflags '$(LDFLAGS)' -o adastra .

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l . | (! grep .) || (echo "gofmt needed on files above" && exit 1)

install:
	go install -ldflags '$(LDFLAGS)' .

clean:
	rm -f adastra
	rm -rf dist/

release-snapshot:
	goreleaser release --snapshot --clean
