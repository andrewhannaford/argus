package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// hashpw prints a bcrypt hash of a password read from stdin (or argv).
func main() {
	pw := ""
	if len(os.Args) > 1 {
		pw = strings.Join(os.Args[1:], " ")
	} else {
		s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		pw = strings.TrimRight(s, "\r\n")
	}
	if pw == "" {
		fmt.Fprintln(os.Stderr, "usage: echo <password> | hashpw")
		os.Exit(1)
	}
	if len(pw) < 12 {
		fmt.Fprintln(os.Stderr, "warning: password is shorter than 12 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}
