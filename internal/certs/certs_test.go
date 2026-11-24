package certs

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestLeafChainsToCAAndCAIsReloaded(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "ca.key")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("ca.key mode = %v, want 0600", fi.Mode().Perm())
	}
	// A second load must reuse the same CA, or the trust install breaks after a restart.
	b, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !a.cert.Equal(b.cert) {
		t.Fatal("reload made a new CA")
	}

	roots := x509.NewCertPool()
	roots.AddCert(b.cert)
	for _, host := range []string{"twitter.com", "", "127.0.0.1"} {
		leaf, err := a.Leaf(host)
		if err != nil {
			t.Fatal(err)
		}
		name := host
		if name == "" {
			name = "localhost"
		}
		if _, err := leaf.Leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: roots}); err != nil {
			t.Errorf("%q: %v", host, err)
		}
	}
}

func TestHalfMissingCAIsAnError(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "ca.key"))
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("cert without key should fail, not make a new CA silently")
	}
}
