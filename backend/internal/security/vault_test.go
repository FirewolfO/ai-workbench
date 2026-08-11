package security

import "testing"

func TestVaultRoundTrip(t *testing.T) {
	vault, err := NewVault("a-secure-test-key-with-at-least-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := vault.Encrypt("sk-secret")
	if err != nil || encrypted == "sk-secret" {
		t.Fatalf("Encrypt() = %q, %v", encrypted, err)
	}
	plain, err := vault.Decrypt(encrypted)
	if err != nil || plain != "sk-secret" {
		t.Fatalf("Decrypt() = %q, %v", plain, err)
	}
}

func TestVaultRejectsShortKey(t *testing.T) {
	if _, err := NewVault("short"); err == nil {
		t.Fatal("expected short key to be rejected")
	}
}
