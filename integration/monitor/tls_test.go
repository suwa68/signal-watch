package main

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

// Characterizes the trust/name boundary used by the unchanged SDK image: the
// ephemeral leaf must need this CA and must validate only api.telegram.org.
func TestEphemeralCertificateTrustAndName(t *testing.T) {
	dir := t.TempDir()
	cert, err := certificates(dir)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "api.telegram.org"}); err != nil {
		t.Fatal(err)
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "example.com"}); err == nil {
		t.Fatal("wrong DNS name accepted")
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: x509.NewCertPool(), DNSName: "api.telegram.org"}); err == nil {
		t.Fatal("leaf trusted without fixture CA")
	}

	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "ca.pem" {
		t.Fatalf("private material persisted: %v %v", files, err)
	}
}
