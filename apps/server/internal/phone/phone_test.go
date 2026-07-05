package phone

import (
	"errors"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantE164   string
		wantRegion string
		wantErr    error
	}{
		{"cn mobile", "+8613800138000", "+8613800138000", "CN", nil},
		{"cn with spaces", " +86 138 0013 8000 ", "+8613800138000", "CN", nil},
		{"us number", "+14155552671", "+14155552671", "US", nil},
		{"hk number", "+85261234567", "+85261234567", "HK", nil},
		{"bare national digits rejected", "13800138000", "", "", ErrInvalid},
		{"too short", "+8612345", "", "", ErrInvalid},
		{"garbage", "not-a-phone", "", "", ErrInvalid},
		{"invalid for region", "+8699999999999", "", "", ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e164, region, err := Normalize(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if e164 != tc.wantE164 || region != tc.wantRegion {
				t.Fatalf("got (%s, %s), want (%s, %s)", e164, region, tc.wantE164, tc.wantRegion)
			}
		})
	}
}

func TestAllowed(t *testing.T) {
	if err := Allowed("CN", nil); err != nil {
		t.Fatalf("empty policy must allow: %v", err)
	}
	if err := Allowed("CN", []string{"CN", "HK"}); err != nil {
		t.Fatalf("listed region must pass: %v", err)
	}
	if err := Allowed("US", []string{"CN"}); !errors.Is(err, ErrRegionNotAllowed) {
		t.Fatalf("unlisted region must fail, got %v", err)
	}
}

func TestNationalNumber(t *testing.T) {
	got, err := NationalNumber("+8613800138000")
	if err != nil {
		t.Fatal(err)
	}
	if got != "13800138000" {
		t.Fatalf("national = %q, want 13800138000", got)
	}
}

func TestNormalizeWithRegion(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		region     string
		wantE164   string
		wantRegion string
		wantErr    error
	}{
		{"bare cn with cn hint", "13800138000", "CN", "+8613800138000", "CN", nil},
		{"bare cn spaced with cn hint", "138 0013 8000", "CN", "+8613800138000", "CN", nil},
		{"bare us with us hint", "4155552671", "US", "+14155552671", "US", nil},
		{"e164 keeps own country despite hint", "+14155552671", "CN", "+14155552671", "US", nil},
		{"bare digits without hint rejected", "13800138000", "", "", "", ErrInvalid},
		{"garbage with hint", "not-a-phone", "CN", "", "", ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e164, region, err := NormalizeWithRegion(tc.input, tc.region)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if e164 != tc.wantE164 || region != tc.wantRegion {
				t.Fatalf("got (%s, %s), want (%s, %s)", e164, region, tc.wantE164, tc.wantRegion)
			}
		})
	}
}

func TestDefaultRegion(t *testing.T) {
	if got := DefaultRegion(nil); got != "" {
		t.Fatalf("no policy => empty, got %q", got)
	}
	if got := DefaultRegion([]string{"CN"}); got != "CN" {
		t.Fatalf("single region => that region, got %q", got)
	}
	if got := DefaultRegion([]string{"CN", "HK"}); got != "" {
		t.Fatalf("multiple regions => empty, got %q", got)
	}
}
