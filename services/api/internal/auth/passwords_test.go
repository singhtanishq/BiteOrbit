package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Demo@1234x")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.Contains(hash, "argon2id") {
		t.Fatalf("hash should be PHC argon2id format, got %q", hash)
	}
	ok, err := VerifyPassword("Demo@1234x", hash)
	if err != nil || !ok {
		t.Fatalf("correct password must verify (ok=%v err=%v)", ok, err)
	}
	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatal("wrong password must not verify")
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same-password-1A")
	b, _ := HashPassword("same-password-1A")
	if a == b {
		t.Fatal("identical passwords must produce different hashes (salt)")
	}
}

func TestPasswordPolicy(t *testing.T) {
	if err := PasswordPolicy("short1A"); err == nil {
		t.Fatal("short password must be rejected")
	}
	if err := PasswordPolicy("alllowercase1"); err == nil {
		t.Fatal("missing upper-case must be rejected")
	}
	if err := PasswordPolicy("NoDigitsHere"); err == nil {
		t.Fatal("missing digit must be rejected")
	}
	if err := PasswordPolicy("GoodPassw0rd"); err != nil {
		t.Fatalf("good password rejected: %v", err)
	}
}
