@echo off
rem Builds PadDeck.exe (requires Go 1.24+ from https://go.dev/dl/)
setlocal
cd /d "%~dp0"
go vet -unsafeptr=false ./... || exit /b 1
go test ./... || exit /b 1
go build -trimpath -ldflags "-H windowsgui -s -w" -o PadDeck.exe . || exit /b 1
echo.
echo Built %CD%\PadDeck.exe
