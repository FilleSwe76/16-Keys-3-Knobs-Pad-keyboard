#!/usr/bin/env sh
# Cross-compiles PadDeck.exe for Windows from Linux/macOS (requires Go 1.24+).
set -e
cd "$(dirname "$0")"
go test ./...
GOOS=windows GOARCH=amd64 go vet -unsafeptr=false ./...
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o PadDeck.exe .
echo "Built $(pwd)/PadDeck.exe"
