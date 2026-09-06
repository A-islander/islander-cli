.PHONY: build test run demo
build:
	go build -o bin/islander .
test:
	go test -race ./...
	go vet ./...
run: build
	./bin/islander tui
demo: build
	python3 scripts/demo.py
