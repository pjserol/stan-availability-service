.PHONY: run test race bench lint

run:
	go run ./cmd/server

test:
	go test ./...

race:
	go test -race ./...

bench:
	go test -run=^$$ -bench=. -benchmem ./...

lint:
	go vet ./...
