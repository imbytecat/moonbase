package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// geetestSign is the HMAC-SHA256(lot_number, key) hex digest required by the
// Geetest v4 validate API as sign_token.
func geetestSign(key, lotNumber string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(lotNumber))
	return hex.EncodeToString(mac.Sum(nil))
}
