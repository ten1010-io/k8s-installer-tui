package ui

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what the app frame says (i18n): keys are the English in the code.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		// the steps of the header (stepTitles)
		"1.Nodes":          "1.노드",
		"2.Groups":         "2.그룹",
		"3.Network":        "3.네트워크",
		"4.K8s":            "4.K8s",
		"4b.Control plane": "4b.컨트롤플레인",
		"5.AIPub":          "5.AIPub",
		"5b.Monitoring":    "5b.모니터링",
		"5c.Python":        "5c.Python",
		"6.AIPub LB":       "6.AIPub LB",
		"7.Certs":          "7.인증서",
		"8.Save":           "8.저장",
		"9.Install":        "9.설치",

		// the footer
		"Ctrl+N next":  "Ctrl+N 다음",
		"Ctrl+P back":  "Ctrl+P 이전",
		"↑↓ move":      "↑↓ 이동",
		"Enter select": "Enter 선택",
		"Ctrl+C quit":  "Ctrl+C 종료",
		"Ctrl+S save":  "Ctrl+S 저장",
		"Cleaning up… (stopping sending the key, Ctrl+C quits now)": "정리 중… (보내던 키를 멈추는 중, Ctrl+C 로 바로 종료)",

		"Save or cancel the item being edited before moving on": "입력 중인 항목을 저장하거나 취소한 뒤 이동하세요",
		"Loading...": "로딩 중...",
	})
}
