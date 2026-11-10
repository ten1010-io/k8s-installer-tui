package plan

import (
	"context"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// P7: AIPub and Harbor are served at hosts under the zone of the site
// (https://<aipub_app_host_prefix>.<aipub_ingress_zone>, and
// <aipub_harbor_ingress_subdomain>.<aipub_ingress_zone>), which has to resolve to
// the ingress VIP the load balancer screen sets (K8sIngressVIP). The check
// resolves both from the control node (getent hosts, read only). DNS is often
// registered after the install, so a miss is a warning, never a failure. With
// separate IPs for ingress and gateway (mode 1) it still compares with the
// ingress VIP: the hosts of the gateway are not in the settings.

// AccessHosts are the hosts AIPub and Harbor are served at; none without a zone.
func AccessHosts(s *state.AppState) []string {
	zone := strings.Trim(strings.TrimSpace(s.AipubIngressZone), ".")
	if zone == "" {
		return nil
	}
	app := strings.TrimSpace(s.AipubAppHostPrefix)
	if app == "" {
		app = "aipub" // the default of aipub-installer
	}
	hosts := []string{app + "." + zone}
	if h := strings.TrimSpace(s.HarborIngressSubdomain); h != "" {
		hosts = append(hosts, h+"."+zone)
	}
	return hosts
}

// CheckDNS is P7.
func CheckDNS(ctx context.Context, o Options, s *state.AppState, run RunFunc) CheckResult {
	vip := strings.TrimSpace(s.K8sIngressVIP)
	hosts := AccessHosts(s)
	switch {
	case !s.AipubEnabled:
		return CheckResult{OK: true, Skipped: true, Lines: []string{"AIPub 을 설치하지 않음"}}
	case vip == "":
		return CheckResult{OK: true, Skipped: true, Lines: []string{"인그레스 VIP 가 정해지지 않음"}}
	case len(hosts) == 0:
		return CheckResult{OK: true, Skipped: true, Lines: []string{"AIPub 도메인 (aipub_ingress_zone) 이 정해지지 않음"}}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	r := CheckResult{OK: true}
	for _, h := range hosts {
		out, _ := run(ctx, o.AnsibleDir(), "getent", "hosts", h)
		var got []string
		for _, l := range strings.Split(string(out), "\n") {
			if f := strings.Fields(l); len(f) > 0 {
				got = append(got, f[0])
			}
		}
		switch {
		case len(got) == 0:
			r.Warn = true
			r.Lines = append(r.Lines, h+": 답 없음 (기대값 인그레스 VIP "+vip+")")
		case contains(got, vip):
			r.Lines = append(r.Lines, h+" → "+vip+" (인그레스 VIP 와 같음)")
		default:
			r.Warn = true
			r.Lines = append(r.Lines, h+": "+strings.Join(got, ", ")+" (기대값 인그레스 VIP "+vip+")")
		}
	}
	if r.Warn {
		r.Lines = append(r.Lines, "DNS 는 설치 뒤에 등록하는 경우가 많아 경고로만 표시합니다. 외부에서 접속하려면 *."+
			strings.Trim(s.AipubIngressZone, ".")+" 가 인그레스 VIP 를 가리켜야 합니다")
	}
	return r
}
