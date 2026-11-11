package fileio

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// FillAuthConfig is --fill-auth-config: it writes the authentication config of the
// JWKS preset whole - the issuer and the CA of the file saving looks at
// (LookupCA) - to the saved site (p) and to the files directory of the installer
// (installerFiles, <root>/ansible/files), so that the copy k8s-installer reads
// and the copy the wizard loads again never differ. A copy that already holds
// it is not touched, so running it again changes nothing. An error, and nothing
// written, when the preset is off, has no issuer, or the CA file is not there or
// not a CA: the CA the config written before holds is not taken for it, as it
// may be another cluster's (a control node rebuilt since).
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
	if ca.PEM == "" || ca.FromSaved {
		msg := "CA 가 없습니다: " + ca.Path + " (aipub-installer setup-cert 가 둡니다. setup-cert 뒤에 다시 실행하세요)"
		if ca.Err != nil {
			msg = "CA 를 읽지 못했습니다: " + ca.Err.Error()
		}
		if ca.FromSaved {
			msg += ". 저장된 인증 설정의 CA 는 이 클러스터의 것인지 알 수 없어 쓰지 않습니다"
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
	site := filepath.Dir(p.Inventory)
	h, err := ReadHistory(site)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", HistoryFile, err)
	}
	var out []string
	for i, t := range targets {
		if cur, err := os.ReadFile(t); err == nil && bytes.Equal(cur, data) {
			out = append(out, "unchanged: "+t)
			continue
		}
		// the site keeps its history; the installer's copy goes to it too, as a
		// copy, after what it replaces (as C1 does)
		var err error
		if i == 0 {
			_, err = writeWithHistory(site, t, data, h)
		} else {
			err = writeInstallerCopy(site, h, t, data)
		}
		if hh, rerr := ReadHistory(site); rerr == nil {
			h = hh
		}
		if err != nil {
			return out, fmt.Errorf("%s: %w", t, err)
		}
		out = append(out, "written: "+t)
	}
	return out, nil
}

// writeInstallerCopy writes the installer's copy of the authentication config,
// keeping what it replaces in the history of the site and recording the copy.
func writeInstallerCopy(site string, h []HistoryEntry, path string, data []byte) error {
	rel := "files/" + state.JWKSConfigFile
	if old, err := os.ReadFile(path); err == nil {
		if keep, _ := keepInstaller(h, rel, old); keep != nil {
			if err := appendHistory(site, *keep); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := writeAtomic(path, data); err != nil {
		return err
	}
	return appendHistory(site, entry(HistoryCopy, rel, data))
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
