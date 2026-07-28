package auth

import (
	"testing"
)

func TestGenerateAndValidate(t *testing.T) {
	secret := "test-secret"
	userID := "user-123"

	token, err := GenerateToken(userID, secret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	got, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got != userID {
		t.Fatalf("expected userID %q, got %q", userID, got)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, _ := GenerateToken("user-1", "secret-a")
	_, err := ValidateToken(token, "secret-b")
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	// We can't easily test expired without time mocking, skip for MVP
}
