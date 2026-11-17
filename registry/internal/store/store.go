// Package store is the SQLite-backed registry of operators and applications.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Application struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	ServiceName     string     `json:"service_name"`
	Owner           string     `json:"owner"`
	TokenPrefix     string     `json:"token_prefix"`
	HealthURL       string     `json:"health_url"`
	Environment     string     `json:"environment"`
	Active          bool       `json:"active"`
	HealthStatus    string     `json:"health_status"`
	HealthCode      int        `json:"health_code"`
	HealthLatencyMs int64      `json:"health_latency_ms"`
	HealthCheckedAt *time.Time `json:"health_checked_at"`
	LastSeen        *time.Time `json:"last_seen"`
	CreatedAt       time.Time  `json:"created_at"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite: serialize writes
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'operator',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS applications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  service_name TEXT UNIQUE NOT NULL,
  owner TEXT DEFAULT '',
  token_hash TEXT NOT NULL,
  token_prefix TEXT NOT NULL,
  health_url TEXT DEFAULT '',
  environment TEXT DEFAULT 'production',
  active INTEGER NOT NULL DEFAULT 1,
  health_status TEXT DEFAULT 'unknown',
  health_code INTEGER DEFAULT 0,
  health_latency_ms INTEGER DEFAULT 0,
  health_checked_at TIMESTAMP,
  last_seen TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_app_token ON applications(token_hash);`)
	return err
}

// ---------------------------- users ----------------------------

func (s *Store) CreateUser(username, passwordHash, role string) (*User, error) {
	res, err := s.db.Exec(`INSERT INTO users(username,password_hash,role) VALUES(?,?,?)`,
		username, passwordHash, role)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, Role: role, CreatedAt: time.Now()}, nil
}

// EnsureUser inserts the user if absent; returns true if it created one.
func (s *Store) EnsureUser(username, passwordHash, role string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	_, err := s.CreateUser(username, passwordHash, role)
	return err == nil, err
}

func (s *Store) PasswordHash(username string) (string, string, bool) {
	var hash, role string
	err := s.db.QueryRow(`SELECT password_hash,role FROM users WHERE username=?`, username).Scan(&hash, &role)
	if err != nil {
		return "", "", false
	}
	return hash, role, true
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id,username,role,created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ------------------------- applications ------------------------

// CreateApp inserts an application and returns it plus the one-time plaintext token.
func (s *Store) CreateApp(name, serviceName, owner, healthURL, env string) (*Application, string, error) {
	if serviceName == "" {
		return nil, "", errors.New("service_name required")
	}
	if env == "" {
		env = "production"
	}
	token, prefix, hash, err := newToken()
	if err != nil {
		return nil, "", err
	}
	res, err := s.db.Exec(`INSERT INTO applications(name,service_name,owner,token_hash,token_prefix,health_url,environment)
		VALUES(?,?,?,?,?,?,?)`, name, serviceName, owner, hash, prefix, healthURL, env)
	if err != nil {
		return nil, "", err
	}
	id, _ := res.LastInsertId()
	app, err := s.GetApp(id)
	if err != nil {
		return nil, "", err
	}
	return app, token, nil
}

func (s *Store) GetApp(id int64) (*Application, error) {
	return s.scanOne(`SELECT `+appCols+` FROM applications WHERE id=?`, id)
}

func (s *Store) ListApps() ([]Application, error) {
	rows, err := s.db.Query(`SELECT ` + appCols + ` FROM applications ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteApp(id int64) error {
	_, err := s.db.Exec(`DELETE FROM applications WHERE id=?`, id)
	return err
}

func (s *Store) SetActive(id int64, active bool) error {
	_, err := s.db.Exec(`UPDATE applications SET active=? WHERE id=?`, b2i(active), id)
	return err
}

// RotateToken issues a new token for an app and returns the plaintext once.
func (s *Store) RotateToken(id int64) (string, error) {
	token, prefix, hash, err := newToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`UPDATE applications SET token_hash=?,token_prefix=? WHERE id=?`, hash, prefix, id)
	return token, err
}

// AppByToken returns the active application matching a plaintext ingest token.
func (s *Store) AppByToken(token string) (*Application, bool) {
	hash := hashToken(token)
	a, err := s.scanOne(`SELECT `+appCols+` FROM applications WHERE token_hash=? AND active=1`, hash)
	if err != nil {
		return nil, false
	}
	return a, true
}

func (s *Store) TouchLastSeen(id int64) {
	_, _ = s.db.Exec(`UPDATE applications SET last_seen=CURRENT_TIMESTAMP WHERE id=?`, id)
}

func (s *Store) UpdateHealth(id int64, status string, code int, latencyMs int64) {
	_, _ = s.db.Exec(`UPDATE applications SET health_status=?,health_code=?,health_latency_ms=?,health_checked_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, code, latencyMs, id)
}

// ------------------------- helpers ------------------------

const appCols = `id,name,service_name,owner,token_prefix,health_url,environment,active,health_status,health_code,health_latency_ms,health_checked_at,last_seen,created_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanApp(sc scanner) (*Application, error) {
	var a Application
	var active int
	var checkedAt, lastSeen sql.NullTime
	if err := sc.Scan(&a.ID, &a.Name, &a.ServiceName, &a.Owner, &a.TokenPrefix, &a.HealthURL,
		&a.Environment, &active, &a.HealthStatus, &a.HealthCode, &a.HealthLatencyMs,
		&checkedAt, &lastSeen, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.Active = active == 1
	if checkedAt.Valid {
		a.HealthCheckedAt = &checkedAt.Time
	}
	if lastSeen.Valid {
		a.LastSeen = &lastSeen.Time
	}
	return &a, nil
}

func (s *Store) scanOne(query string, args ...any) (*Application, error) {
	return scanApp(s.db.QueryRow(query, args...))
}

func newToken() (token, prefix, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	token = "pvgo_" + hex.EncodeToString(buf)
	prefix = token[:11] + "…"
	hash = hashToken(token)
	return
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Ping verifies the DB is reachable.
func (s *Store) Ping() error {
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}
