package fileio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// FillAuthConfig is --fill-auth-config: it writes the authentication config of the
// JWKS preset whole - the issuer and the CA, found as saving finds it (LookupCA) -
// to the saved site (p) and to the files directory of the installer
// (installerFiles, <root>/ansible/files), so that the copy k8s-installer reads
// and the copy the wizard loads again never differ. A copy that already holds
// it is not touched, so running it again changes nothing. An error, and nothing
// written, when the preset is off, has no issuer, or there is no CA.
//
// The plan runs it after setup-cert of aipub-installer has put the CA on the
// nodes, before the update-cluster.yml that applies the preset.
func FillAuthConfig(p Paths, s *state.AppState, installerFiles string) ([]string, error) {
	if !s.AipubEnabled {
		return nil, errors.New("AIPub 을 설치하지 않는 사이트입니다 (JWKS issuer 는 AIPub)")
	}
	if !s.ControlPlane.JWKS {
		return nil, errors.New("AIPub 인증 (JWKS) 프리셋이 꺼져 있습니다")
	}
	if s.JWKSIssuer() == "" {
		return nil, errors.New("JWKS issuer 가 없습니다 (aipub_ingress_zone 또는 issuer URL)")
	}
	ca := LookupCA(s)
	if ca.PEM == "" {
		msg := "CA 가 없습니다: " + ca.Path + " (aipub-installer setup-cert 가 둡니다. setup-cert 뒤에 다시 실행하세요)"
		if ca.Err != nil {
			msg = "CA 를 읽지 못했습니다: " + ca.Err.Error()
		}
		return nil, errors.New(msg)
	}
	data := AuthConfig(s, ca.PEM)
	if data == nil {
		return nil, errors.New("authentication-config.yaml 을 만들지 못했습니다 (템플릿이 바뀌었는지 확인)")
	}

	targets := []string{p.AuthConfig()}
	if installerFiles != "" {
		installed := filepath.Join(installerFiles, filepath.FromSlash(state.JWKSConfigFile))
		if !samePath(installed, targets[0]) {
			targets = append(targets, installed)
		}
	}
	var out []string
	if ca.Err != nil {
		out = append(out, "CA 파일은 쓸 수 없어 저장된 CA 를 씀: "+ca.Err.Error())
	}
	for _, t := range targets {
		if cur, err := os.ReadFile(t); err == nil && bytes.Equal(cur, data) {
			out = append(out, "unchanged: "+t)
			continue
		}
		if err := writeWithBackup(t, data); err != nil {
			return out, fmt.Errorf("%s: %w", t, err)
		}
		out = append(out, "written: "+t)
	}
	return out, nil
}

func samePath(a, b string) bool {
	if aa, err := filepath.Abs(a); err == nil {
		a = aa
	}
	if bb, err := filepath.Abs(b); err == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
