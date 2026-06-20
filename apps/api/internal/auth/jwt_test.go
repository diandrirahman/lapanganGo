package auth

import "testing"

func TestTokenServiceGenerateAndParse(t *testing.T) {
	tokenService := NewTokenService("test-secret", 1)
	user := UserResponse{
		ID:     "user-1",
		Email:  "owner@example.com",
		Role:   "OWNER",
		Name:   "Owner",
		Status: "ACTIVE",
	}

	token, err := tokenService.Generate(user)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got %v", err)
	}

	claims, err := tokenService.Parse(token)
	if err != nil {
		t.Fatalf("expected token parse to succeed, got %v", err)
	}

	if claims.UserID != user.ID {
		t.Fatalf("expected user id %q, got %q", user.ID, claims.UserID)
	}
	if claims.Email != user.Email {
		t.Fatalf("expected email %q, got %q", user.Email, claims.Email)
	}
	if claims.Role != user.Role {
		t.Fatalf("expected role %q, got %q", user.Role, claims.Role)
	}
	if claims.Subject != user.ID {
		t.Fatalf("expected subject %q, got %q", user.ID, claims.Subject)
	}
}

func TestTokenServiceParseRejectsWrongSecret(t *testing.T) {
	tokenService := NewTokenService("test-secret", 1)
	token, err := tokenService.Generate(UserResponse{
		ID:     "user-1",
		Email:  "owner@example.com",
		Role:   "OWNER",
		Name:   "Owner",
		Status: "ACTIVE",
	})
	if err != nil {
		t.Fatalf("expected token generation to succeed, got %v", err)
	}

	wrongTokenService := NewTokenService("wrong-secret", 1)
	if _, err := wrongTokenService.Parse(token); err == nil {
		t.Fatal("expected parse with wrong secret to fail")
	}
}
