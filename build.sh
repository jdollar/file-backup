#!/usr/bin/env sh

#CGO_ENABLED false for alpine
CGO_ENABLED=0 go build -o build/backup cmd/backup/main.go

