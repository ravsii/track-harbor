PHONY: build
build: clean
	mkdir -p bin
	go build -o ./bin/meshvault .

PHONY: clean
clean:
	rm -rf ./bin/
