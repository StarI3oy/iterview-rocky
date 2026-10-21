package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Params is an Argon2id profile. RecommendedParams follows the OWASP
// Password Storage Cheat Sheet (argon2id, 19 MiB, t=2, p=1).
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// RecommendedParams is the profile used for real passwords.
func RecommendedParams() Params {
	return Params{Memory: 19 * 1024, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32}
}

// FastParams is intentionally weak and exists only so tests finish quickly.
func FastParams() Params {
	return Params{Memory: 8, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}
}

func (p Params) validate() error {
	if p.Memory < 8 || p.Memory > 256*1024 {
		return errors.New("argon2 memory")
	}
	if p.Time < 1 || p.Time > 10 {
		return errors.New("argon2 time")
	}
	if p.Threads < 1 || p.Threads > 8 {
		return errors.New("argon2 threads")
	}
	if p.SaltLen < 16 || p.SaltLen > 64 {
		return errors.New("argon2 salt")
	}
	if p.KeyLen < 16 || p.KeyLen > 64 {
		return errors.New("argon2 key")
	}
	return nil
}

// ValidatePassword applies a length policy (NIST 800-63B: length, no composition rules).
func ValidatePassword(password string) error {
	if strings.ContainsRune(password, 0) {
		return errors.New("пароль содержит недопустимый символ")
	}
	n := utf8.RuneCountInString(password)
	if strings.TrimSpace(password) == "" || n < 12 {
		return errors.New("пароль короче 12 символов")
	}
	if n > 128 {
		return errors.New("пароль длиннее 128 символов")
	}
	if _, ok := blockedPasswords[strings.ToLower(strings.TrimSpace(password))]; ok {
		return errors.New("этот пароль слишком частый")
	}
	return nil
}

func hashPassword(password string, p Params) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return encodePHC(p, salt, sum), nil
}

func verifyPassword(encoded, password string) error {
	p, salt, want, err := decodePHC(encoded)
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrBadPassword
	}
	return nil
}

func encodePHC(p Params, salt, sum []byte) string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	)
}

func decodePHC(s string) (Params, []byte, []byte, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return Params{}, nil, nil, errors.New("неизвестный формат хеша")
	}
	var p Params
	for _, field := range strings.Split(parts[3], ",") {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			return Params{}, nil, nil, errors.New("неизвестный формат хеша")
		}
		n, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return Params{}, nil, nil, errors.New("неизвестный формат хеша")
		}
		switch key {
		case "m":
			p.Memory = uint32(n)
		case "t":
			p.Time = uint32(n)
		case "p":
			if n > 255 {
				return Params{}, nil, nil, errors.New("argon2 threads")
			}
			p.Threads = uint8(n)
		default:
			return Params{}, nil, nil, errors.New("неизвестный формат хеша")
		}
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, errors.New("неизвестный формат хеша")
	}
	sum, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, errors.New("неизвестный формат хеша")
	}
	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(sum))
	if err := p.validate(); err != nil {
		return Params{}, nil, nil, err
	}
	return p, salt, sum, nil
}

var blockedPasswords = map[string]struct{}{
	"password1234":     {},
	"passwordpassword": {},
	"qwertyqwerty":     {},
	"123456789012":     {},
	"1234567890123":    {},
	"administrator":    {},
	"letmeinplease":    {},
	"iloveyou1234":     {},
	"prepapp12345":     {},
	"парольпароль12":   {},
}
