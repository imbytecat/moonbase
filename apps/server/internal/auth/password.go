package auth

import "github.com/alexedwards/argon2id"

// OWASP cheat-sheet cost (m=19MiB, t=2, p=1); argon2id owns the PHC format.
var argonParams = &argon2id.Params{
	Memory:      19 * 1024,
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// DummyHash is verified against when the email doesn't exist, so unknown
// emails cost the same wall time as wrong passwords (anti user-enumeration).
// Any valid hash with the production params works; the input never matches.
var DummyHash = mustHash("timing-equalization-dummy")

func mustHash(password string) string {
	hash, err := argon2id.CreateHash(password, argonParams)
	if err != nil {
		panic(err)
	}
	return hash
}

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argonParams)
}

// VerifyPassword returns false (not an error) for a mismatch.
func VerifyPassword(password, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, hash)
}
