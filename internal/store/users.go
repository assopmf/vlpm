package store

import (
	"database/sql"
	"errors"
	"strings"
)

var ErrNotFound = errors.New("introuvable")

const userCols = `id, matricule, nom, prenom, email, password_hash, role, actif, must_change, derniere_connexion, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var derniere sql.NullString
	err := row.Scan(&u.ID, &u.Matricule, &u.Nom, &u.Prenom, &u.Email, &u.PasswordHash,
		&u.Role, &u.Actif, &u.MustChange, &derniere, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.DerniereConnexion = ns(derniere)
	return &u, nil
}

func (s *Store) UserByID(id int64) (*User, error) {
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) UserByMatricule(matricule string) (*User, error) {
	return scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE matricule = ? COLLATE NOCASE`,
		strings.TrimSpace(matricule)))
}

func (s *Store) ListUsers(inclureInactifs bool) ([]User, error) {
	q := `SELECT ` + userCols + ` FROM users`
	if !inclureInactifs {
		q += ` WHERE actif = 1`
	}
	q += ` ORDER BY nom, prenom`
	rows, err := s.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(u *User) error {
	res, err := s.DB.Exec(`INSERT INTO users (matricule, nom, prenom, email, password_hash, role, actif, must_change)
		VALUES (?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(u.Matricule), u.Nom, u.Prenom, u.Email, u.PasswordHash, u.Role, u.Actif, u.MustChange)
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateUser(u *User) error {
	_, err := s.DB.Exec(`UPDATE users SET matricule=?, nom=?, prenom=?, email=?, role=?, actif=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`,
		strings.TrimSpace(u.Matricule), u.Nom, u.Prenom, u.Email, u.Role, u.Actif, u.ID)
	return err
}

func (s *Store) SetPassword(userID int64, hash string, mustChange bool) error {
	_, err := s.DB.Exec(`UPDATE users SET password_hash=?, must_change=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`, hash, mustChange, userID)
	return err
}

func (s *Store) TouchLogin(userID int64) error {
	_, err := s.DB.Exec(`UPDATE users SET derniere_connexion=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`, userID)
	return err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
