package auth

import (
	"errors"
	"strings"
	"testing"
)

// fastParams keeps tests quick; production uses DefaultArgon2Params.
var fastParams = Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", fastParams)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(correct) = %v, %v; want true, nil", ok, err)
	}
	ok, err = VerifyPassword("wrong password", hash)
	if err != nil || ok {
		t.Fatalf("VerifyPassword(wrong) = %v, %v; want false, nil", ok, err)
	}
}

func TestHashPasswordUsesUniqueSalt(t *testing.T) {
	a, _ := HashPassword("same", fastParams)
	b, _ := HashPassword("same", fastParams)
	if a == b {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyPasswordRespectsStoredParams(t *testing.T) {
	// A hash created with different params must still verify, so params can
	// be upgraded without breaking existing users.
	p := fastParams
	p.Time = 2
	hash, _ := HashPassword("pw-123456", p)
	if ok, err := VerifyPassword("pw-123456", hash); err != nil || !ok {
		t.Fatalf("verify with t=2 hash: %v, %v", ok, err)
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	cases := []string{
		"",
		"plain",
		"$argon2i$v=19$m=1024,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=18$m=1024,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=1024,t=1,p=1$!!!$a2V5",
		"$argon2id$v=19$m=1024,t=1,p=1$c2FsdA$",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("VerifyPassword(%q) err = %v; want ErrMalformedHash", c, err)
		}
	}
}

func TestTokenAndHash(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("tokens are not unique")
	}
	if len(a) != 43 { // 32 bytes base64url without padding
		t.Fatalf("token length = %d; want 43", len(a))
	}
	if string(HashToken(a)) != string(HashToken(a)) {
		t.Fatal("HashToken is not deterministic")
	}
	if len(HashToken(a)) != 32 {
		t.Fatal("HashToken must be 32 bytes (BINARY(32) column)")
	}
}
