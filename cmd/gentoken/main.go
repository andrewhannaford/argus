package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// gentoken prints a random 256-bit agent token (hex).
func main() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hex.EncodeToString(b))
}
