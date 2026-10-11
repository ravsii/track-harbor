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
	go run ./server/cmd/server/

migrate-up:
	$(GOOSE) up

migrate-down:
	$(GOOSE) down


migrate-dry-run:
	@echo "=== CURRENT STATUS ==="
	$(GOOSE) status
	@echo ""

.PHONY: build clean run run-server migrate-up migrate-down migrate-status migrate-up-one migrate-reset migrate-dry-run
