.PHONY: build test lint clean install

APP_NAME := leech
VERSION := $(shell grep 'Version' app/version.go | cut -d'"' -f2)

build:
	go build -o $(APP_NAME) .

test:
	go test -v -race -failfast ./...

lint:
	golangci-lint run

clean:
	rm -f $(APP_NAME)
	rm -f *.part

install:
	go install .
