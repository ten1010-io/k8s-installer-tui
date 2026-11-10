package ui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
)

// withCA points the CA of the JWKS preset at a file holding pemText (nothing
// there when it is empty) for the test.
func withCA(t *testing.T, pemText string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	if pemText != "" {
		if err := os.WriteFile(path, []byte(pemText), 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := fileio.DefaultCAPath
	fileio.DefaultCAPath = path
	t.Cleanup(func() { fileio.DefaultCAPath = old })
}

func newTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "aipub-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
