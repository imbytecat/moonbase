// Package phone contains number formatting helpers needed by integration
// drivers. Business validation policy stays in the base server; this package is
// only for provider-specific formatting of already-normalized E.164 numbers.
package phone

import (
	"errors"
	"fmt"

	"github.com/nyaruka/phonenumbers"
)

var ErrInvalid = errors.New("invalid phone number")

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
