# Makefile for tachyon-installer

VERSION ?= 1.0.0

.PHONY: all build-all build-windows build-linux build-darwin test clean

all: build-all

test:
	go test -v ./...

build-all:
	go run scripts/build.go -v $(VERSION)

build-windows:
	go run scripts/build.go -v $(VERSION) -os windows

build-linux:
	go run scripts/build.go -v $(VERSION) -os linux

build-darwin:
	go run scripts/build.go -v $(VERSION) -os darwin

clean:
	rm -rf dist/
