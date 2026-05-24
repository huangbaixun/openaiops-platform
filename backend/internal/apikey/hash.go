package apikey

import "golang.org/x/crypto/bcrypt"

// Cost 10 is bcrypt default; raise to 12 in prod (config-driven) later.
const hashCost = 10

func Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), hashCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Verify(plain, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}
