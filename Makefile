.PHONY: build run test vet deps

build:
	go build ./...

run:
	go run .

test:
	go test ./...

vet:
	go vet ./...

deps:
	go mod download
