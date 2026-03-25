package auth

import (
	"testing"
)

func TestHashPassword_ProducesValidHash(t *testing.T) {
	hash, err := HashPassword("mysecretpassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if hash == "mysecretpassword" {
		t.Error("hash must not equal the plaintext password")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !CheckPassword(hash, "correct-password") {
		t.Error("CheckPassword returned false for the correct password")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword returned true for the wrong password")
	}
}
