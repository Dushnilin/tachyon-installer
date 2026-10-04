package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func writeKey(t *testing.T, passphrase string) (string, gossh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var pemBytes []byte
	if passphrase == "" {
		b, err := gossh.MarshalPrivateKey(priv, "")
		if err != nil {
			t.Fatal(err)
		}
		pemBytes = pemEncode(b)
	} else {
		b, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
		if err != nil {
			t.Fatal(err)
		}
		pemBytes = pemEncode(b)
	}
	path := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sp, _ := gossh.NewPublicKey(pub)
	return path, sp
}

func TestLoadSignerPlainKey(t *testing.T) {
	path, pub := writeKey(t, "")
	s, err := LoadSigner(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(s.PublicKey().Marshal()) != string(pub.Marshal()) {
		t.Fatal("loaded key does not match")
	}
}

func TestLoadSignerEncryptedKey(t *testing.T) {
	path, _ := writeKey(t, "secret")
	if _, err := LoadSigner(path, ""); err == nil || !strings.Contains(err.Error(), "парольной фразой") {
		t.Fatalf("expected passphrase hint, got %v", err)
	}
	if _, err := LoadSigner(path, "wrong"); err == nil {
		t.Fatal("wrong passphrase must fail")
	}
	if _, err := LoadSigner(path, "secret"); err != nil {
		t.Fatalf("right passphrase failed: %v", err)
	}
}

func TestLoadSignerMissingFile(t *testing.T) {
	if _, err := LoadSigner(filepath.Join(t.TempDir(), "nope"), ""); err == nil {
		t.Fatal("missing file must fail")
	}
}

func TestAuthMethodsOrder(t *testing.T) {
	path, _ := writeKey(t, "")
	old := PrivateKeyPath
	defer func() { PrivateKeyPath = old }()

	PrivateKeyPath = path
	if n := len(authMethods("pw")); n != 2 {
		t.Fatalf("key + password expected, got %d methods", n)
	}
	PrivateKeyPath = filepath.Join(t.TempDir(), "missing")
	if n := len(authMethods("pw")); n != 1 {
		t.Fatalf("unreadable key must fall back to password only, got %d", n)
	}
}
