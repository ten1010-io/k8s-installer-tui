package fileio

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// DefaultCAPath is where the CA of the JWKS preset is read from when the site
// names no other file: the place setup-cert of aipub-installer puts it on every
// node, the control node this runs on among them. A variable so that a test, or
// --jwks-ca, can point it at a file of its own.
var DefaultCAPath = state.JWKSCAPath

// CA is the CA the authentication config of the JWKS preset is written with.
type CA struct {
	// PEM is the CA, one certificate or more; empty when there is none yet
	PEM string
	// Path is the file looked at
	Path string
	// FromSaved: PEM is what the authentication config written before holds,
	// as the file at Path is not there (or is not a CA)
	FromSaved bool
	// Err: the file at Path is there but could not be read or is not a CA
	Err error
}

// LookupCA finds the CA of the JWKS preset: the file the site named on the
// control plane screen, else DefaultCAPath; when that is not there, the CA the
// authentication config written before holds (read back by LoadAll), so that
// saving again keeps it. PEM is empty when there is neither: the config is then
// not written at all (see AuthConfig).
func LookupCA(s *state.AppState) CA {
	path := strings.TrimSpace(s.ControlPlane.JWKSCAFile)
	if path == "" {
		path = DefaultCAPath
	}
	ca := CA{Path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		pemText, perr := ParseCA(data)
		if perr == nil {
			ca.PEM = pemText
			return ca
		}
		ca.Err = fmt.Errorf("%s: %w", path, perr)
	} else if !errors.Is(err, fs.ErrNotExist) {
		ca.Err = err
	}
	if saved := strings.TrimSpace(s.ControlPlane.JWKSCA); saved != "" {
		ca.PEM, ca.FromSaved = saved, true
	}
	return ca
}

// ParseCA checks that data is a CA certificate in PEM - one CERTIFICATE block
// or more and nothing else, every one of them a certificate x509 parses - and
// returns it as it is to be inlined: verbatim, without the space around it.
// Anything else in the file (a private key above all) is refused rather than
// copied, as the config goes to every control plane node in the clear. Text
// between or around the blocks (the subject lines some tools write) is dropped by
// writing the blocks again.
func ParseCA(data []byte) (string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest := []byte(text)
	var blocks []*pem.Block
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			return "", fmt.Errorf("PEM 블록 %q 가 있습니다 (CA 인증서만 넣습니다)", b.Type)
		}
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			return "", fmt.Errorf("인증서를 읽지 못했습니다: %w", err)
		}
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return "", errors.New("PEM 인증서가 없습니다")
	}
	if strings.Contains(string(rest), "-----BEGIN") {
		return "", errors.New("PEM 블록을 끝까지 읽지 못했습니다")
	}
	// pem.Decode passes over a block it cannot read (an END line of another
	// type, a broken line) and goes on to the next: such a block is still in the
	// text, and it may be a key. Refused, as anything not a certificate is
	if n := strings.Count(text, "-----BEGIN "); n != len(blocks) {
		return "", fmt.Errorf("PEM 블록 %d 개 중 %d 개만 인증서로 읽었습니다 (깨진 블록이 있습니다)", n, len(blocks))
	}
	trimmed := strings.TrimSpace(text)
	var again strings.Builder
	for _, b := range blocks {
		again.Write(pem.EncodeToMemory(&pem.Block{Type: b.Type, Bytes: b.Bytes}))
	}
	// verbatim when the file is the blocks alone, as k8s-jwks copies it; written
	// again otherwise
	if onlyBlocks(trimmed) {
		return trimmed, nil
	}
	return strings.TrimSpace(again.String()), nil
}

// onlyBlocks reports whether text is PEM blocks and nothing between them.
func onlyBlocks(text string) bool {
	in := false
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "-----BEGIN "):
			in = true
		case strings.HasPrefix(l, "-----END "):
			in = false
		case !in && l != "":
			return false
		}
	}
	return true
}
