.PHONY: all test lint fmt generate clean

all: test

test:
	go test -v -race ./...

lint:
	golangci-lint run --config=.github/golangci.yml

fmt:
	gofmt -s -w .
	goimports -w -local github.com/craftedsignal/kql-parser .

generate:
	@echo "Generating parser from ANTLR grammar..."
	@command -v antlr4 >/dev/null 2>&1 || { echo "antlr4 is required but not installed. Install with: brew install antlr"; exit 1; }
	antlr4 -Dlanguage=Go -visitor -listener -o . KQLLexer.g4 KQLParser.g4
	@echo "Done."

clean:
	go clean -testcache

benchmark:
	go test -bench=. -benchmem ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
