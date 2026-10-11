.PHONY: build run run-server run-server-watch css-build css-watch

css-build:
	npm run css:build

css-watch:
	npm run css:watch

build:
	mkdir -p .local-server
	go build -o .local-server/trackharbor-server ./cmd/server/

run: run-server

run-server: build
	./.local-server/trackharbor-server

run-server-watch:
	python3 scripts/run-server-watch.py
