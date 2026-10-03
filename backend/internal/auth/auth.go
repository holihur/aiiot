package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidToken = errors.New("invalid token")

// Token kinds: business users and administrators are isolated audiences.
const (
	TokenKindUser  = "user"
	TokenKindAdmin = "admin"
)

// Claims is the JWT payload for an authenticated platform user.
type Claims struct {
	UserID     uint   `json:"uid"`
	Username   string `json:"username"`
	SystemRole string `json:"role"`
	Kind       string `json:"kind"`
	// TokenVersion invalidates tokens when it no longer matches the user's
	// current version (password change / revoke-all).
	TokenVersion int `json:"tv"`
	jwt.RegisteredClaims
}

// TokenService issues and verifies user access tokens.
type TokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

func (s *TokenService) Issue(userID uint, username, role string, version int) (string, time.Time, error) {
	return s.issue(userID, username, role, TokenKindUser, version)
}

// IssueAdmin mints a token for the administration audience.
func (s *TokenService) IssueAdmin(userID uint, username string, version int) (string, time.Time, error) {
	return s.issue(userID, username, "admin", TokenKindAdmin, version)
}

func (s *TokenService) issue(userID uint, username, role, kind string, version int) (string, time.Time, error) {
	exp := time.Now().Add(s.ttl)
	claims := Claims{
		UserID:       userID,
		Username:     username,
		SystemRole:   role,
		Kind:         kind,
		TokenVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
			Subject:   username,
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	return tok, exp, err
}

func (s *TokenService) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
