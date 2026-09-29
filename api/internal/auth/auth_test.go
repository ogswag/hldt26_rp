package auth

import (
	"bytes"
	"testing"
)

func TestTokens(t *testing.T) {
	a, hashA, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) != 43 {
		t.Fatalf("tokens %q %q", a, b)
	}
	if !bytes.Equal(HashToken(a), hashA) || len(hashA) != 32 {
		t.Fatal("hash must be the SHA-256 of the token")
	}
}

func TestIPPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.77":          "203.0.113.0/24",
		"2001:db8:1234:5678::1": "2001:db8:1234::/48",
		"::ffff:10.1.2.3":       "10.1.2.0/24",
		"not-an-ip":             "",
	} {
		if got := IPPrefix(in); got != want {
			t.Errorf("IPPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("demo-user", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "demo-user") {
		t.Fatal("check")
	}
	if CheckPassword(hash, "demo-admin") {
		t.Fatal("wrong password matched")
	}
}
