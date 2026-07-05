package phone

import (
	"strings"
	"testing"
)

// FuzzNormalize asserts the two invariants that matter for untrusted login /
// registration input: parsing arbitrary bytes never panics, and any number it
// accepts is canonical — re-normalizing the returned E.164 yields the exact
// same E.164 (idempotence), with a real region (never "" or "ZZ").
func FuzzNormalize(f *testing.F) {
	for _, seed := range []string{
		"+8613800138000", "+14155552671", "+442071838750",
		"13800138000", " +81 90-1234-5678 ", "", "+", "+++",
		"not a phone", "+0000000000000000000", "01189998819991197253",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		e164, region, err := Normalize(input)
		if err != nil {
			return // rejected input is fine; we only constrain accepted output
		}
		if region == "" || region == "ZZ" {
			t.Fatalf("Normalize(%q) accepted with bogus region %q", input, region)
		}
		if !strings.HasPrefix(e164, "+") {
			t.Fatalf("Normalize(%q) = %q, want a leading +", input, e164)
		}
		// Idempotence: the canonical form must be a fixed point.
		e164b, regionb, err := Normalize(e164)
		if err != nil {
			t.Fatalf("re-normalizing canonical %q failed: %v", e164, err)
		}
		if e164b != e164 || regionb != region {
			t.Fatalf("not idempotent: Normalize(%q) = (%q,%q), want (%q,%q)",
				e164, e164b, regionb, e164, region)
		}
	})
}

// FuzzNationalNumber pins the round-trip contract SMS drivers depend on:
// NationalNumber never panics, and for anything Normalize accepts it returns
// non-empty digits that are a suffix of the E.164 form.
func FuzzNationalNumber(f *testing.F) {
	f.Add("+8613800138000")
	f.Add("+14155552671")
	f.Add("garbage")

	f.Fuzz(func(t *testing.T, input string) {
		e164, _, err := Normalize(input)
		if err != nil {
			_, _ = NationalNumber(input) // must not panic on junk either
			return
		}
		nsn, err := NationalNumber(e164)
		if err != nil {
			t.Fatalf("NationalNumber(%q) failed on accepted number: %v", e164, err)
		}
		if nsn == "" || !strings.HasSuffix(e164, nsn) {
			t.Fatalf("NationalNumber(%q) = %q, want non-empty suffix of E.164", e164, nsn)
		}
	})
}
