.PHONY: build test run demo release
VERSION ?= dev
build:
	go build -ldflags "-X github.com/A-islander/islander-cli/internal/cli.Version=$(VERSION)" -o bin/islander ./cmd/islander
test:
	go test -race ./...
	go vet ./...
run: build
	./bin/islander tui
demo: build
	python3 scripts/demo.py
release:
	python3 scripts/package.py --version $(VERSION) --appimage
