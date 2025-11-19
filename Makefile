# Makefile for Kubikles

.PHONY: dev build install-wails

# Ensure GOPATH/bin is in PATH
GOPATH := $(shell go env GOPATH)
WAILS := $(GOPATH)/bin/wails

dev:
	$(WAILS) dev

build:
	$(WAILS) build

install-wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@latest
