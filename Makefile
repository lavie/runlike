# Go build, test and release builds. Run `make help` for the targets.
SHELL := bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBUILD := CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)'
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 windows/amd64 windows/arm64
FUZZTIME ?= 30s

.PHONY: help build test fuzz lint cross clean
help:
	@echo 'build  build/runlike for this machine'
	@echo 'test   go vet + go test ./... (live suites need DOCKER_CONTEXT, see test/)'
	@echo 'fuzz   each Fuzz target for FUZZTIME (default 30s)'
	@echo 'lint   gofmt + go vet'
	@echo 'cross  static binaries for $(PLATFORMS) in dist/, with SHA256SUMS'

build:
	$(GOBUILD) -o build/runlike ./cmd/runlike

test:
	go vet ./...
	go test ./...

fuzz:
	@set -e; for f in $$(go test -list '^Fuzz' . | grep '^Fuzz'); do \
		go test -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) . ; done

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

cross:
	rm -rf dist && mkdir -p dist
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "dist/runlike-$$os-$$arch$$ext"; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) -o dist/runlike-$$os-$$arch$$ext ./cmd/runlike; done
	cd dist && shasum -a 256 runlike-* > SHA256SUMS

clean:
	rm -rf build dist
