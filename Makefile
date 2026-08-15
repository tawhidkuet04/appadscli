VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tawhidkuet04/asacli/cmd.Version=$(VERSION)

.PHONY: build test lint install clean release-snapshot

build:
	go build -ldflags '$(LDFLAGS)' -o asacli .

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l . | (! grep .) || (echo "gofmt needed on files above" && exit 1)

install:
	go install -ldflags '$(LDFLAGS)' .

clean:
	rm -f asacli
	rm -rf dist/

release-snapshot:
	goreleaser release --snapshot --clean
