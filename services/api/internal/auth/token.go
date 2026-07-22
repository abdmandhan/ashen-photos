package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 720 * time.Hour // 30 days

type TokenService struct {
	secret []byte
}

func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: []byte(secret)}
}

// Issue mints a signed JWT whose subject is the user id.
func (t *TokenService) Issue(userID string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Parse validates the token and returns the user id (subject).
func (t *TokenService) Parse(tokenStr string) (string, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{},
		func(tok *jwt.Token) (any, error) {
			if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
			}
			return t.secret, nil
		})
	if err != nil {
		return "", err
	}
	claims, ok := tok.Claims.(*jwt.RegisteredClaims)
	if !ok || !tok.Valid {
		return "", fmt.Errorf("invalid token")
	}
	return claims.Subject, nil
}
