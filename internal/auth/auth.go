// Package auth gère les mots de passe et les jetons de session.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/assopmf/vlpm/internal/store"
)

var (
	ErrIdentifiants    = errors.New("matricule ou mot de passe incorrect")
	ErrCompteInactif   = errors.New("compte désactivé")
	ErrSessionInvalide = errors.New("session invalide ou expirée")
)

// DureeSession : durée de validité d'un jeton. Volontairement longue car les
// agents utilisent l'application depuis un terminal mobile en patrouille.
const DureeSession = 12 * time.Hour

func HashPassword(motDePasse string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(motDePasse), bcrypt.DefaultCost)
	return string(h), err
}

// ValiderMotDePasse impose un minimum : 10 caractères, avec chiffre et lettre.
func ValiderMotDePasse(mdp string) error {
	if len([]rune(mdp)) < 10 {
		return errors.New("le mot de passe doit faire au moins 10 caractères")
	}
	var chiffre, lettre bool
	for _, r := range mdp {
		switch {
		case unicode.IsDigit(r):
			chiffre = true
		case unicode.IsLetter(r):
			lettre = true
		}
	}
	if !chiffre || !lettre {
		return errors.New("le mot de passe doit contenir au moins une lettre et un chiffre")
	}
	return nil
}

// hashFactice est un vrai hash bcrypt, calculé une fois au démarrage, dont le
// coût de vérification est identique à celui d'un compte réel.
var hashFactice = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("aucun-compte-n-a-ce-mot-de-passe"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return h
}()

type Service struct {
	st *store.Store
}

func New(st *store.Store) *Service { return &Service{st: st} }

// Login vérifie les identifiants et ouvre une session.
func (s *Service) Login(matricule, motDePasse, userAgent, ip string) (*store.User, string, error) {
	u, err := s.st.UserByMatricule(matricule)
	if err != nil {
		// On compare quand même contre un hash factice valide : sans cela, un
		// matricule inexistant répondrait bien plus vite qu'un mauvais mot de
		// passe, ce qui permettrait d'énumérer les comptes au chronomètre.
		bcrypt.CompareHashAndPassword(hashFactice, []byte(motDePasse))
		return nil, "", ErrIdentifiants
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(motDePasse)); err != nil {
		return nil, "", ErrIdentifiants
	}
	if !u.Actif {
		return nil, "", ErrCompteInactif
	}

	token, err := s.creerSession(u.ID, userAgent, ip)
	if err != nil {
		return nil, "", err
	}
	_ = s.st.TouchLogin(u.ID)
	return u, token, nil
}

// VerifierMotDePasse contrôle un mot de passe sans ouvrir de session : utilisé
// pour reconfirmer l'identité avant une opération sensible.
func (s *Service) VerifierMotDePasse(u *store.User, motDePasse string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(motDePasse)) == nil
}

// creerSession génère un jeton aléatoire de 256 bits. Seul son SHA-256 est
// stocké : une copie de la base ne permet pas de rejouer les sessions.
func (s *Service) creerSession(userID int64, userAgent, ip string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("génération du jeton: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expire := time.Now().UTC().Add(DureeSession).Format(time.RFC3339)

	if _, err := s.st.DB.Exec(`INSERT INTO sessions (token, user_id, expires_at, user_agent, ip)
		VALUES (?,?,?,?,?)`, hashToken(token), userID, expire, tronquer(userAgent, 200),
		tronquer(ip, 64)); err != nil {
		return "", err
	}
	return token, nil
}

// Utilisateur résout un jeton de session vers l'utilisateur correspondant.
func (s *Service) Utilisateur(token string) (*store.User, error) {
	if token == "" {
		return nil, ErrSessionInvalide
	}
	var userID int64
	var expire string
	err := s.st.DB.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token = ?`,
		hashToken(token)).Scan(&userID, &expire)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalide
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, expire)
	if err != nil || time.Now().UTC().After(t) {
		_ = s.Logout(token)
		return nil, ErrSessionInvalide
	}
	u, err := s.st.UserByID(userID)
	if err != nil {
		return nil, ErrSessionInvalide
	}
	if !u.Actif {
		return nil, ErrCompteInactif
	}
	return u, nil
}

func (s *Service) Logout(token string) error {
	_, err := s.st.DB.Exec(`DELETE FROM sessions WHERE token = ?`, hashToken(token))
	return err
}

// LogoutTous ferme toutes les sessions d'un utilisateur (changement de mot de
// passe, désactivation du compte, terminal perdu).
func (s *Service) LogoutTous(userID int64) error {
	_, err := s.st.DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// PurgerSessions supprime les sessions expirées. Appelé périodiquement.
func (s *Service) PurgerSessions() (int64, error) {
	res, err := s.st.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// HashJeton expose le condensat d'un jeton : la liste des sessions le compare
// à celui stocké pour marquer l'appareil courant, sans jamais manipuler le
// jeton en clair hors de l'authentification.
func HashJeton(token string) string { return hashToken(token) }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ComparerConstant compare deux chaînes sans fuite temporelle.
func ComparerConstant(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func tronquer(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// MotDePasseAleatoire génère un mot de passe initial lisible pour un agent.
func MotDePasseAleatoire() (string, error) {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 14)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	// Garantit la présence d'un chiffre exigée par ValiderMotDePasse.
	out[len(out)-1] = "23456789"[int(b[0])%8]
	return string(out), nil
}
