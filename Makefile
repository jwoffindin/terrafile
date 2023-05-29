# Makefile for golang 1.20 project with 'setup' 'build', and 'test' tasks

setup:
	@echo "Installing dependencies..."
	go mod download

build:
	@echo "Building..."
	go build -mod=mod -o terrafile

test:
	@echo "Testing..."
	go test -mod=readonly -v ./...
