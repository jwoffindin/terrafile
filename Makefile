# Makefile for golang 1.20 project with 'setup' 'build', and 'test' tasks

setup:
	@echo "Installing dependencies..."
	go mod download

build:
	@echo "Building..."
	GO111MODULE=on go build -mod=mod -o terrafile

test:
	@echo "Testing..."
	GO111MODULE=on go test -mod=readonly -v ./...
