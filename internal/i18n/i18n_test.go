package i18n

import "testing"

func TestTSaysTheCurrentLanguage(t *testing.T) {
	defer Set(Current())
	Register(KO, map[string]string{
		"archive %s not found: %v": "아카이브 %[1]s 를 찾을 수 없습니다: %[2]v",
		"Next":                     "다음",
	})
	Set(EN)
	if got := T("archive %s not found: %v", "/a.tgz", "gone"); got != "archive /a.tgz not found: gone" {
		t.Errorf("en: %q", got)
	}
	if got := T("100% sure"); got != "100% sure" {
		t.Errorf("no args, a lone %%: %q", got)
	}
	Set(KO)
	if got := T("archive %s not found: %v", "/a.tgz", "gone"); got != "아카이브 /a.tgz 를 찾을 수 없습니다: gone" {
		t.Errorf("ko: %q", got)
	}
	if got := T("Next"); got != "다음" {
		t.Errorf("ko, no args: %q", got)
	}
	if got := T("not translated %d", 3); got != "not translated 3" {
		t.Errorf("ko, no translation: %q", got)
	}
}

func TestRegisterRefusesTwoTranslations(t *testing.T) {
	Register(KO, map[string]string{"same": "같음"})
	Register(KO, map[string]string{"same": "같음"}) // the same again is fine
	defer func() {
		if recover() == nil {
			t.Error("two translations of one key taken")
		}
	}()
	Register(KO, map[string]string{"same": "다름"})
}

func TestFromArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		env  string
		want Lang
		bad  bool
	}{
		{nil, "", EN, false},
		{[]string{"--install"}, "", EN, false},
		{[]string{"--lang", "ko"}, "", KO, false},
		{[]string{"-lang=ko"}, "", KO, false},
		{[]string{"--lang=KO"}, "", KO, false},
		{[]string{"--lang", "en"}, "ko", EN, false},
		{nil, "ko", KO, false},
		{nil, "ko_KR.UTF-8", KO, false},
		{nil, "en_US.UTF-8", EN, false},
		{[]string{"--inventory", "x", "--lang", "ko", "--install"}, "", KO, false},
		{[]string{"--lang", "en", "--lang", "ko"}, "", KO, false},
		{[]string{"--lang=ko", "--lang", "en"}, "ko", EN, false},
		{[]string{"--", "--lang", "ko"}, "", EN, false},
		{[]string{"--language", "ko"}, "", EN, false},
		{[]string{"--lang"}, "", EN, true},
		{[]string{"--lang", "fr"}, "", EN, true},
	} {
		got, err := FromArgs(c.args, c.env)
		if got != c.want || (err != nil) != c.bad {
			t.Errorf("%q env %q: %s %v", c.args, c.env, got, err)
		}
	}
}
