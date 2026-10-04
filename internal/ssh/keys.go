package ssh

import (
	"errors"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// PrivateKeyPath is an optional private key used for authentication before the password.
// It is set once from the wizard.
var PrivateKeyPath string

// LoadSigner reads a private key. If the key is encrypted, passphrase is used.
func LoadSigner(path, passphrase string) (gossh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := gossh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	var missing *gossh.PassphraseMissingError
	if errors.As(err, &missing) {
		if passphrase == "" {
			return nil, errors.New("ключ защищён парольной фразой: введите её в поле «Пароль»")
		}
		return gossh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	return nil, err
}

// DefaultKeyPaths lists the usual private key locations of the current user.
func DefaultKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		p := filepath.Join(home, ".ssh", n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// authMethods builds the auth chain: explicit key, default keys (only when no password
// is given), then the password.
func authMethods(password string) []gossh.AuthMethod {
	var methods []gossh.AuthMethod
	if PrivateKeyPath != "" {
		if s, err := LoadSigner(PrivateKeyPath, password); err == nil {
			methods = append(methods, gossh.PublicKeys(s))
		}
	} else if password == "" {
		for _, p := range DefaultKeyPaths() {
			if s, err := LoadSigner(p, ""); err == nil {
				methods = append(methods, gossh.PublicKeys(s))
			}
		}
	}
	return append(methods, gossh.Password(password))
}
