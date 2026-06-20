package auth

import "testing"

func TestNormalizeEmail(t *testing.T) {
	got := normalizeEmail("  OWNER@Example.COM  ")
	want := "owner@example.com"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestToUserResponseOmitsPasswordHash(t *testing.T) {
	phone := "08123456789"
	user := User{
		ID:           "user-1",
		Name:         "Owner",
		Email:        "owner@example.com",
		Phone:        &phone,
		PasswordHash: "secret-hash",
		Role:         "OWNER",
		Status:       "ACTIVE",
	}

	response := toUserResponse(user)

	if response.ID != user.ID {
		t.Fatalf("expected id %q, got %q", user.ID, response.ID)
	}
	if response.Email != user.Email {
		t.Fatalf("expected email %q, got %q", user.Email, response.Email)
	}
	if response.Phone == nil || *response.Phone != phone {
		t.Fatalf("expected phone %q, got %v", phone, response.Phone)
	}
	if response.Role != user.Role {
		t.Fatalf("expected role %q, got %q", user.Role, response.Role)
	}
}
