package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

func SignedURL(secret []byte, method, purpose, key string, expires time.Duration) string {
	exp := time.Now().Add(expires).Unix()
	q := url.Values{"exp": {strconv.FormatInt(exp, 10)}, "sig": {Signature(secret, method, purpose, key, exp)}}
	return "/api/files/" + purpose + "/" + key + "?" + q.Encode()
}

func Signature(secret []byte, method, purpose, key string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%s\n%d", method, purpose, key, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(secret []byte, method, purpose, key string, exp int64, sig string) bool {
	if time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(Signature(secret, method, purpose, key, exp)), []byte(sig))
}
