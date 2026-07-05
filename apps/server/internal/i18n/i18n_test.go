package i18n

import "testing"

// TestCatalogParity keeps every locale's key set identical to the default, so
// a half-added translation is a red test, not a silent English fallback.
func TestCatalogParity(t *testing.T) {
	base := catalog[Default]
	for loc, m := range catalog {
		for k := range base {
			if _, ok := m[k]; !ok {
				t.Errorf("locale %s is missing key %q", loc, k)
			}
		}
		for k := range m {
			if _, ok := base[k]; !ok {
				t.Errorf("locale %s has key %q absent from the default locale", loc, k)
			}
		}
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		userLocale string
		accept     string
		want       Locale
	}{
		{"en", "", En},
		{"zh-CN", "", ZhCN},
		{"", "en-US,en;q=0.9", En},
		{"", "zh-CN,zh;q=0.9", ZhCN},
		{"", "fr-FR", Default},
		{"", "", Default},
		{"invalid", "en-US", En},
		{"en", "zh-CN", En},
	}
	for _, c := range cases {
		if got := Resolve(c.userLocale, c.accept); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", c.userLocale, c.accept, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	if got := T(En, AuthCodeSubject); got != "Your verification code" {
		t.Errorf("T(En, subject) = %q", got)
	}
	if got := T("fr", AuthCodeSubject); got != T(Default, AuthCodeSubject) {
		t.Errorf("unknown locale should fall back to the default translation")
	}
	if got := T(En, "no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key should return the key, got %q", got)
	}
	if got := T(En, AuthCodeBody, "123456"); got != "Your verification code is 123456. It expires in 5 minutes.\n" {
		t.Errorf("T with args = %q", got)
	}
}
