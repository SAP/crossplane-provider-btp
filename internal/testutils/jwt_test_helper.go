package testutils

import (
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
)

var hmacSampleSecret = []byte("my_test_key")

func JwtToken(ts timeSupplier, m ...JwtModifier) string {
	now := ts()
	claims := jwt.MapClaims{}
	for _, f := range m {
		key, val := f(now)
		claims[key] = val.Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		panic("could not build jwt" + err.Error())
	}

	return tokenString
}

func JwtTokenWithIssuer(issuer string, ts timeSupplier, m ...JwtModifier) string {
	now := ts()
	claims := jwt.MapClaims{}
	claims["iss"] = issuer
	for _, f := range m {
		key, val := f(now)
		claims[key] = val.Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		panic("could not build jwt" + err.Error())
	}

	return tokenString
}

type timeSupplier func() time.Time

var now time.Time

func init() {
	now = time.Now()
}

func Now() time.Time {
	return now
}

func Epoch() time.Time {
	return time.Unix(0, 0)
}

type JwtModifier func(time.Time) (string, time.Time)

func NotBefore(d time.Duration) func(time.Time) (string, time.Time) {
	return func(t time.Time) (string, time.Time) {
		return "nbf", t.Add(d)
	}
}

func IssuedAt(d time.Duration) func(time.Time) (string, time.Time) {
	return func(t time.Time) (string, time.Time) {
		return "iat", t.Add(d)
	}
}

func ExpiresAt(d time.Duration) func(time.Time) (string, time.Time) {
	return func(t time.Time) (string, time.Time) {
		return "exp", t.Add(d)
	}
}
