package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey int

const identityKey ctxKey = 1

const (
	RoleGuest = "guest"
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Identity is the caller of a request. SessionID and CSRF are empty for guests.
type Identity struct {
	UserID    uuid.UUID
	Email     string
	Role      string
	SessionID uuid.UUID
	CSRF      string
}

func Guest() Identity {
	return Identity{Role: RoleGuest}
}

func (id Identity) IsGuest() bool {
	return id.Role == "" || id.Role == RoleGuest
}

func (id Identity) IsAdmin() bool {
	return id.Role == RoleAdmin
}

// NewToken returns a random session token for the cookie and its SHA-256 for the database.
func NewToken() (string, []byte, error) {
	s, err := randomString()
	if err != nil {
		return "", nil, err
	}
	return s, HashToken(s), nil
}

// HashToken is the database key of a session token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// NewCSRF returns a random CSRF token bound to one session.
func NewCSRF() (string, error) {
	return randomString()
}

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth.random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// IPPrefix keeps the /24 of an IPv4 or the /48 of an IPv6 address, so the session list never stores a full address.
func IPPrefix(addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return (&net.IPNet{IP: v4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(48, 128)), Mask: net.CIDRMask(48, 128)}).String()
}

func HashPassword(pw string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
	if err != nil {
		return "", fmt.Errorf("auth.hash: %w", err)
	}
	return string(b), nil
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

func FromContext(ctx context.Context) Identity {
	if v, ok := ctx.Value(identityKey).(Identity); ok {
		return v
	}
	return Guest()
}

func FromRequest(r *http.Request) Identity {
	return FromContext(r.Context())
}
