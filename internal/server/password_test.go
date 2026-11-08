package server

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordMatches(t *testing.T) {
	pw := "ArgusVerifyTestPassword123456789012"
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if !passwordMatches(pw, string(h)) {
		t.Fatal("bcrypt hash did not match correct password")
	}
	if passwordMatches("wrong", string(h)) {
		t.Fatal("bcrypt hash matched wrong password")
	}
	if !passwordMatches(pw, pw) {
		t.Fatal("legacy plaintext fallback failed")
	}
	if passwordMatches(pw, "plaintextdifferent") {
		t.Fatal("legacy plaintext fallback matched wrong value")
	}
}
