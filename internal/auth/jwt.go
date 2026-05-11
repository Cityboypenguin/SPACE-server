package auth

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	TokenType string `json:"tokenType"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil
	}
	return []byte(secret)
}

func tokenExpirationMinutes(envKey string, defaultMinutes int) int {
	if envExp := os.Getenv(envKey); envExp != "" {
		if parsed, err := strconv.Atoi(envExp); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultMinutes
}

func jwtIssuer() string  { return os.Getenv("JWT_ISSUER") }
func jwtAudience() string { return os.Getenv("JWT_AUDIENCE") }

func generateTokenWithType(id int64, role string, tokenType string, expirationMinutes int) (string, error) {
	secret := jwtSecret()
	if len(secret) == 0 {
		return "", errors.New("JWT_SECRET environment variable must be set")
	}

	registered := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expirationMinutes) * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	if iss := jwtIssuer(); iss != "" {
		registered.Issuer = iss
	}
	if aud := jwtAudience(); aud != "" {
		registered.Audience = jwt.ClaimStrings{aud}
	}

	claims := Claims{
		ID:               id,
		Role:             role,
		TokenType:        tokenType,
		RegisteredClaims: registered,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func validateTokenWithType(tokenString string, expectedType string) (*Claims, error) {
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	actualType := claims.TokenType
	if actualType == "" {
		actualType = "access"
	}
	if actualType != expectedType {
		return nil, errors.New("invalid token type")
	}

	return claims, nil
}

func GenerateAccessToken(id int64, role string) (string, error) {
	expirationMinutes := tokenExpirationMinutes("JWT_EXPIRATION_MINUTES", 60)
	return generateTokenWithType(id, role, "access", expirationMinutes)
}

func GenerateRefreshToken(id int64, role string) (string, error) {
	expirationMinutes := tokenExpirationMinutes("JWT_REFRESH_EXPIRATION_MINUTES", 60*24*14)
	return generateTokenWithType(id, role, "refresh", expirationMinutes)
}

func ValidateAccessToken(tokenString string) (*Claims, error) {
	return validateTokenWithType(tokenString, "access")
}

func ValidateRefreshToken(tokenString string) (*Claims, error) {
	return validateTokenWithType(tokenString, "refresh")
}

func ValidateToken(tokenString string) (*Claims, error) {
	secret := jwtSecret()
	if len(secret) == 0 {
		return nil, errors.New("JWT_SECRET environment variable must be set")
	}

	parserOpts := []jwt.ParserOption{}
	if iss := jwtIssuer(); iss != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(iss))
	}
	if aud := jwtAudience(); aud != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(aud))
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	}, parserOpts...)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
