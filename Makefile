DB_PATH=meshtune.db
MIGRATIONS_DIR=./migrations

GOOSE=goose -dir $(MIGRATIONS_DIR) sqlite3 $(DB_PATH)

build: clean
	mkdir -p bin
	go build -o ./bin/trackharbor ./cmd/cli/

clean:
	rm -rf ./bin/
	rm -rf ./tmp_music/*

run: clean
	go run ./cmd/cli/

run-server:
	mkdir -p .local-server
	go build -o .local-server/trackharbor-server ./server/cmd/server/
	./.local-server/trackharbor-server

run-server-watch:
	python3 scripts/run-server-watch.py

migrate-up:
	$(GOOSE) up

migrate-down:
	$(GOOSE) down


migrate-dry-run:
	@echo "=== CURRENT STATUS ==="
	$(GOOSE) status
	@echo ""

.PHONY: build clean run run-server run-server-watch migrate-up migrate-down migrate-status migrate-up-one migrate-reset migrate-dry-run
