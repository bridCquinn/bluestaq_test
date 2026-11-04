.PHONY: run test clean help

help:
	@echo "Available lifecycle commands:"
	@echo "  make run   - Compile and execute the REST engine with structured JSON telemetry logs"
	@echo "  make test  - Run the table-driven multi-tenancy verification test suite"
	@echo "  make clean - Standardize codebase formatting and tidy module requirements"

run:
	go run main.go

test:
	go test -v ./...

clean:
	go fmt ./...
	go mod tidy
