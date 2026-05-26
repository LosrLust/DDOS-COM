package core

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

type User struct {
	ID        int64
	Username  string
	Password  []byte
	Salt      []byte
	Rank      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidAuth  = errors.New("invalid credentials")
)

func ConnectDB(path string) error {
	var err error
	DB, err = sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(1)
	if err := DB.Ping(); err != nil {
		return err
	}
	return migrate()
}

func migrate() error {
	_, err := DB.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password BLOB NOT NULL,
		salt BLOB NOT NULL,
		rank TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}

	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count == 0 {
		salt := makeSalt()
		hash := hashPass("admin", salt)
		_, err = DB.Exec(`INSERT INTO users (username, password, salt, rank) VALUES (?, ?, ?, ?)`,
			"admin", hash, salt, "admin")
		if err != nil {
			return err
		}
		log.Println("  [*] Default admin created (admin/admin)")
	}
	return nil
}

func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}

func hashPass(pw string, salt []byte) []byte {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(pw))
	return h.Sum(nil)
}

func makeSalt() []byte {
	s := make([]byte, 32)
	rand.Read(s)
	return s
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Auth(username, password string) (*User, error) {
	u, err := GetUser(username)
	if err != nil {
		return nil, ErrInvalidAuth
	}
	if !bytesEq(hashPass(password, u.Salt), u.Password) {
		return nil, ErrInvalidAuth
	}
	return u, nil
}

func GetUser(username string) (*User, error) {
	row := DB.QueryRow(`SELECT id, username, password, salt, rank, created_at, updated_at FROM users WHERE username = ?`, username)
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Salt, &u.Rank, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func CreateUser(username, password, rank string) error {
	salt := makeSalt()
	_, err := DB.Exec(`INSERT INTO users (username, password, salt, rank) VALUES (?, ?, ?, ?)`,
		username, hashPass(password, salt), salt, rank)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func DeleteUser(username string) error {
	r, err := DB.Exec(`DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func ListUsers() ([]*User, error) {
	rows, err := DB.Query(`SELECT id, username, password, salt, rank, created_at, updated_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.Salt, &u.Rank, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &u)
	}
	return out, nil
}

func UpdatePassword(username, newPass string) error {
	salt := makeSalt()
	_, err := DB.Exec(`UPDATE users SET password = ?, salt = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`,
		hashPass(newPass, salt), salt, username)
	return err
}

func SetRank(username, rank string) error {
	_, err := DB.Exec(`UPDATE users SET rank = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, rank, username)
	return err
}
