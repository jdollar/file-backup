#!/usr/bin/env sh

#CGO_ENABLED false for alpine
CGO_ENABLED=0 go build -o build/backup cmd/dropbox-backup/main.go

