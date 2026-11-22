//go:build !windows

package main

import "fmt"

// PadDeck is a Windows application. This stub lets the portable parts
// (key codes, layout, keypad protocols) build and run their tests on any OS.
func main() {
	fmt.Println("PadDeck runs on Windows only.")
}
