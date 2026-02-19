package auth

import (
	"testing"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-chars!!"
	token, err := GenerateToken(1, "admin", "admin", secret, 24)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", claims.UserID)
	}
	if claims.Username != "admin" {
		t.Errorf("expected Username admin, got %s", claims.Username)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := GenerateToken(1, "admin", "admin", "secret-one-that-is-long-enough!!", 24)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	_, err = ValidateToken(token, "wrong-secret-that-is-long-enough")
	if err == nil {
		t.Error("expected error validating token with wrong secret")
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	_, err := ValidateToken("", "any-secret")
	if err == nil {
		t.Error("expected error validating empty token")
	}
}
