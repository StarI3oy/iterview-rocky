package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	CookieName = "prep_session"

	maxFails   = 5
	failWindow = 15 * time.Minute
	lockFor    = 15 * time.Minute
	sessionTTL = 7 * 24 * time.Hour
)

var (
	ErrBadPassword = errors.New("неверный пароль")
	ErrNoPassword  = errors.New("пароль ещё не задан")
	ErrPasswordSet = errors.New("пароль уже задан")
)

// RateLimitError is returned when login attempts from one address are paused.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e RateLimitError) Error() string {
	return "слишком много попыток, подождите"
}

// Guard stores an Argon2id password hash and session hashes in a file that is
// separate from the interview databases and the OpenRouter key.
type Guard struct {
	path     string
	params   Params
	mu       sync.Mutex
	hash     string
	sessions map[string]time.Time
	attempts map[string]attempt
	now      func() time.Time
}

type attempt struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

type fileData struct {
	Version      int             `json:"version"`
	KDF          string          `json:"kdf"`
	PasswordHash string          `json:"password_hash"`
	Sessions     []storedSession `json:"sessions"`
}

type storedSession struct {
	Hash    string    `json:"hash"`
	Expires time.Time `json:"expires"`
}

// Open reads the auth file. A missing file means no password yet.
// A corrupt file is returned as an error and left untouched.
func Open(path string, params Params) (*Guard, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	g := &Guard{
		path:     path,
		params:   params,
		sessions: map[string]time.Time{},
		attempts: map[string]attempt{},
		now:      time.Now,
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	var data fileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("файл пароля повреждён (%s), базы не изменены: %w", path, err)
	}
	if data.Version != 1 || (data.PasswordHash != "" && data.KDF != "argon2id") {
		return nil, fmt.Errorf("файл пароля %s имеет неизвестную версию, базы не изменены", path)
	}
	if data.PasswordHash != "" {
		if _, _, _, err := decodePHC(data.PasswordHash); err != nil {
			return nil, fmt.Errorf("хеш пароля не читается, базы не изменены: %w", err)
		}
	}
	g.hash = data.PasswordHash
	now := g.clock()
	for _, s := range data.Sessions {
		if s.Hash == "" || !s.Expires.After(now) {
			continue
		}
		g.sessions[s.Hash] = s.Expires
	}
	_ = os.Chmod(path, 0o600)
	return g, nil
}

// Path is the auth file location.
func (g *Guard) Path() string { return g.path }

// PasswordSet reports whether a hash is stored.
func (g *Guard) PasswordSet() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hash != ""
}

// ApplyEnv sets or checks PREP_PASSWORD. reset replaces the hash.
// The interview databases and the API key are not read here.
func ApplyEnv(g *Guard, password string, reset bool) (string, error) {
	if password == "" {
		if !g.PasswordSet() {
			return "пароль не задан — API с других устройств закрыт. Задайте его в браузере на этом компьютере или через PREP_PASSWORD", nil
		}
		return "защита паролем включена", nil
	}
	if !g.PasswordSet() {
		if err := g.SetPassword(password); err != nil {
			return "", err
		}
		return "пароль сохранён как Argon2id-хеш. Базы и ключ OpenRouter не менялись. Уберите PREP_PASSWORD из окружения, чтобы он не оставался в памяти процесса", nil
	}
	if reset {
		if err := g.SetPassword(password); err != nil {
			return "", err
		}
		return "пароль заменён, старые сессии закрыты. Базы и ключ OpenRouter не менялись. Уберите PREP_PASSWORD и PREP_PASSWORD_RESET из окружения", nil
	}
	if err := g.Verify(password); err != nil {
		return "", errors.New("PREP_PASSWORD не совпадает с сохранённым хешем. Файл пароля, базы и ключ OpenRouter не изменены. Для замены задайте PREP_PASSWORD_RESET=1")
	}
	return "PREP_PASSWORD совпал с хешем. Уберите переменную из окружения, чтобы пароль не оставался в памяти процесса", nil
}

// SetPassword replaces the hash and drops sessions. On failure the previous hash stays.
func (g *Guard) SetPassword(password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	encoded, err := hashPassword(password, g.params)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.storeHashLocked(encoded)
}

// EnablePassword stores the first password. It refuses to replace an existing one.
func (g *Guard) EnablePassword(ip, password string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.gateAttemptLocked(ip); err != nil {
		return err
	}
	if g.hash != "" {
		return ErrPasswordSet
	}
	if err := ValidatePassword(password); err != nil {
		return err
	}
	encoded, err := hashPassword(password, g.params)
	if err != nil {
		return err
	}
	if err := g.storeHashLocked(encoded); err != nil {
		return err
	}
	delete(g.attempts, ip)
	return nil
}

// Verify checks the password and does not change the file.
func (g *Guard) Verify(password string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hash == "" {
		return ErrNoPassword
	}
	if err := verifyPassword(g.hash, password); err != nil {
		return ErrBadPassword
	}
	return nil
}

// Login checks the password and returns a new session token.
func (g *Guard) Login(ip, password string) (string, time.Time, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.gateAttemptLocked(ip); err != nil {
		return "", time.Time{}, err
	}
	if g.hash == "" {
		return "", time.Time{}, ErrNoPassword
	}
	if err := verifyPassword(g.hash, password); err != nil {
		g.noteFailLocked(ip)
		return "", time.Time{}, ErrBadPassword
	}
	delete(g.attempts, ip)
	return g.startSessionLocked()
}

// ChangePassword checks the current password, stores a new hash, and drops every session.
func (g *Guard) ChangePassword(ip, current, next string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.gateAttemptLocked(ip); err != nil {
		return err
	}
	if g.hash == "" {
		return ErrNoPassword
	}
	if err := verifyPassword(g.hash, current); err != nil {
		g.noteFailLocked(ip)
		return ErrBadPassword
	}
	if current == next {
		return errors.New("новый пароль совпадает с текущим")
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	encoded, err := hashPassword(next, g.params)
	if err != nil {
		return err
	}
	if err := g.storeHashLocked(encoded); err != nil {
		return err
	}
	delete(g.attempts, ip)
	return nil
}

// Authenticate reports whether the token is an unexpired session.
func (g *Guard) Authenticate(token string) bool {
	if len(token) < 20 || len(token) > 128 {
		return false
	}
	sum := hashToken(token)
	g.mu.Lock()
	defer g.mu.Unlock()
	exp, ok := g.sessions[sum]
	if !ok {
		return false
	}
	if !exp.After(g.clock()) {
		delete(g.sessions, sum)
		_ = g.saveLocked()
		return false
	}
	return true
}

// Logout drops one session.
func (g *Guard) Logout(token string) {
	if token == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.sessions, hashToken(token))
	_ = g.saveLocked()
}

func (g *Guard) storeHashLocked(encoded string) error {
	prevHash := g.hash
	prevSessions := copySessions(g.sessions)
	g.hash = encoded
	g.sessions = map[string]time.Time{}
	if err := g.saveLocked(); err != nil {
		g.hash = prevHash
		g.sessions = prevSessions
		return err
	}
	return nil
}

func (g *Guard) startSessionLocked() (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	exp := g.clock().Add(sessionTTL)
	g.sessions[hashToken(token)] = exp
	if err := g.saveLocked(); err != nil {
		delete(g.sessions, hashToken(token))
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func (g *Guard) gateAttemptLocked(ip string) error {
	a := g.attempts[ip]
	now := g.clock()
	if a.lockedUntil.After(now) {
		return RateLimitError{RetryAfter: a.lockedUntil.Sub(now)}
	}
	return nil
}

func (g *Guard) noteFailLocked(ip string) {
	now := g.clock()
	a := g.attempts[ip]
	if a.windowStart.IsZero() || now.Sub(a.windowStart) > failWindow {
		a = attempt{windowStart: now}
	}
	a.fails++
	if a.fails >= maxFails {
		a.lockedUntil = now.Add(lockFor)
		a.fails = 0
		a.windowStart = now
	}
	g.attempts[ip] = a
}

func (g *Guard) saveLocked() error {
	data := fileData{
		Version:      1,
		KDF:          "argon2id",
		PasswordHash: g.hash,
		Sessions:     make([]storedSession, 0, len(g.sessions)),
	}
	now := g.clock()
	for hash, exp := range g.sessions {
		if !exp.After(now) {
			continue
		}
		data.Sessions = append(data.Sessions, storedSession{Hash: hash, Expires: exp.UTC()})
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(g.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".prep_auth-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, g.path); err != nil {
		return err
	}
	cleanup = false
	return os.Chmod(g.path, 0o600)
}

func (g *Guard) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func copySessions(in map[string]time.Time) map[string]time.Time {
	out := make(map[string]time.Time, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
