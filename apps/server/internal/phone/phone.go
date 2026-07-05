// Package phone normalizes and validates phone numbers via libphonenumber
// (nyaruka/phonenumbers). Storage format is always E.164; region policy
// (which countries are accepted) is a business setting enforced here.
package phone

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

var (
	ErrInvalid           = errors.New("invalid phone number")
	ErrRegionNotAllowed  = errors.New("phone number region not supported")
	errUnparseableRegion = errors.New("cannot determine phone region")
)

// Normalize parses input (expected E.164, e.g. +8613800138000) and returns
// canonical E.164 plus the ISO region ("CN"). Strict: numbers that aren't
// valid for their region are rejected, so garbage never reaches the DB.
func Normalize(input string) (e164 string, region string, err error) {
	return NormalizeWithRegion(input, "")
}

// NormalizeWithRegion is Normalize with a fallback country for inputs that lack
// a country code (bare national numbers like "13800138000"). E.164 inputs (a
// leading "+") ignore defaultRegion and keep their own country; an empty
// defaultRegion reproduces Normalize's strict E.164-only behavior. This lets a
// single-region deployment accept domestic numbers without the "+86" prefix,
// while multi-region deployments (ambiguous country) still require E.164.
func NormalizeWithRegion(input, defaultRegion string) (e164 string, region string, err error) {
	num, err := phonenumbers.Parse(strings.TrimSpace(input), defaultRegion)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", "", ErrInvalid
	}
	region = phonenumbers.GetRegionCodeForNumber(num)
	if region == "" || region == "ZZ" {
		return "", "", errUnparseableRegion
	}
	return phonenumbers.Format(num, phonenumbers.E164), region, nil
}

// Allowed enforces the region policy: an empty list means no restriction.
func Allowed(region string, allowedRegions []string) error {
	if len(allowedRegions) == 0 {
		return nil
	}
	if slices.Contains(allowedRegions, region) {
		return nil
	}
	return ErrRegionNotAllowed
}

// DefaultRegion returns the sole allowed region when the policy pins exactly
// one country — the region to assume for bare national numbers so users can
// omit the country code. Zero or multiple allowed regions yield "" (callers
// then require E.164, since the intended country is ambiguous).
func DefaultRegion(allowedRegions []string) string {
	if len(allowedRegions) == 1 {
		return allowedRegions[0]
	}
	return ""
}

// NationalNumber returns the digits without country code ("13800138000") —
// what mainland-China SMS providers expect for domestic sends.
func NationalNumber(e164 string) (string, error) {
	num, err := phonenumbers.Parse(e164, "")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return phonenumbers.GetNationalSignificantNumber(num), nil
}

// RegionOf reports the ISO region of an already-normalized E.164 number.
func RegionOf(e164 string) string {
	num, err := phonenumbers.Parse(e164, "")
	if err != nil {
		return ""
	}
	return phonenumbers.GetRegionCodeForNumber(num)
}
