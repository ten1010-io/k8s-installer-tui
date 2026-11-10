package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// dnsAnswers answers getent hosts by host name; a host not given has no answer
// (getent exits 2 and prints nothing).
func dnsAnswers(answers map[string]string, asked *[]string) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if name != "getent" || len(args) != 2 || args[0] != "hosts" {
			return nil, errors.New("unexpected command " + name)
		}
		*asked = append(*asked, args[1])
		if a, ok := answers[args[1]]; ok {
			return []byte(a + "   " + args[1] + "\n"), nil
		}
		return nil, errors.New("exit status 2")
	}
}

func TestCheckDNS(t *testing.T) {
	s := aipubState() // zone example.com, VIP 192.168.0.100
	var asked []string

	r := CheckDNS(context.Background(), testOptions(), s, dnsAnswers(map[string]string{
		"aipub.example.com": "192.168.0.100", "aipub-harbor.example.com": "192.168.0.100"}, &asked))
	if !r.OK || r.Warn || r.Skipped || strings.Join(asked, " ") != "aipub.example.com aipub-harbor.example.com" ||
		r.Lines[0] != "aipub.example.com → 192.168.0.100 (인그레스 VIP 와 같음)" {
		t.Errorf("match: %+v asked %v", r, asked)
	}

	r = CheckDNS(context.Background(), testOptions(), s, dnsAnswers(map[string]string{
		"aipub.example.com": "10.0.0.5", "aipub-harbor.example.com": "192.168.0.100"}, &asked))
	if !r.OK || !r.Warn || r.Lines[0] != "aipub.example.com: 10.0.0.5 (기대값 인그레스 VIP 192.168.0.100)" ||
		!strings.Contains(r.Lines[len(r.Lines)-1], "설치 뒤에 등록하는 경우가 많아") {
		t.Errorf("mismatch: %+v", r)
	}

	r = CheckDNS(context.Background(), testOptions(), s, dnsAnswers(nil, &asked))
	if !r.OK || !r.Warn || r.Lines[0] != "aipub.example.com: 답 없음 (기대값 인그레스 VIP 192.168.0.100)" ||
		r.Lines[1] != "aipub-harbor.example.com: 답 없음 (기대값 인그레스 VIP 192.168.0.100)" {
		t.Errorf("no answer: %+v", r)
	}

	// the site's own prefix and subdomain, and a zone written with a dot
	s.AipubAppHostPrefix, s.HarborIngressSubdomain, s.AipubIngressZone = "ai", "reg", "lab.example.com."
	asked = nil
	CheckDNS(context.Background(), testOptions(), s, dnsAnswers(nil, &asked))
	if strings.Join(asked, " ") != "ai.lab.example.com reg.lab.example.com" {
		t.Errorf("hosts: %v", asked)
	}
}

func TestCheckDNSSkipped(t *testing.T) {
	var asked []string
	run := dnsAnswers(nil, &asked)
	off := aipubState()
	off.AipubEnabled = false
	noVIP := aipubState()
	noVIP.K8sIngressVIP = ""
	noZone := aipubState()
	noZone.AipubIngressZone = ""
	for name, c := range map[string]struct{ s, want string }{"off": {"off", "AIPub 을 설치하지 않음"},
		"no VIP": {"noVIP", "인그레스 VIP 가 정해지지 않음"}, "no zone": {"noZone", "aipub_ingress_zone"}} {
		st := map[string]*state.AppState{"off": off, "noVIP": noVIP, "noZone": noZone}[c.s]
		r := CheckDNS(context.Background(), testOptions(), st, run)
		if !r.OK || !r.Skipped || !strings.Contains(r.Lines[0], c.want) {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if len(asked) != 0 {
		t.Errorf("asked DNS although skipped: %v", asked)
	}
}
