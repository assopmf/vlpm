package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// Politique de second facteur, choisie par chaque commune.
const (
	TOTPDesactive   = "desactive"
	TOTPFacultatif  = "facultatif"
	TOTPObligatoire = "obligatoire_chefs"

	CleTOTPMode = "totp_mode"
)

var modesTOTP = map[string]bool{
	TOTPDesactive: true, TOTPFacultatif: true, TOTPObligatoire: true,
}

var (
	ErrTOTPRequis       = errors.New("code d'authentification requis")
	ErrTOTPInvalide     = errors.New("code d'authentification incorrect")
	ErrTOTPRejeu        = errors.New("ce code a déjà été utilisé")
	ErrModeTOTPInvalide = errors.New(
		"mode inconnu : valeurs acceptées desactive, facultatif, obligatoire_chefs")
)

func (s *Store) ModeTOTP() string {
	m := s.Setting(CleTOTPMode, TOTPDesactive)
	if !modesTOTP[m] {
		return TOTPDesactive
	}
	return m
}

func (s *Store) DefinirModeTOTP(mode string) error {
	if !modesTOTP[mode] {
		return ErrModeTOTPInvalide
	}
	return s.SetSetting(CleTOTPMode, mode)
}

// TOTPExigePour indique si le compte doit obligatoirement porter un second
// facteur. Les agents en sont exclus même en mode obligatoire : le leur
// imposer à chaque prise de service serait une gêne quotidienne pour un gain
// faible, leurs droits se limitant à prendre et rendre un véhicule.
func (s *Store) TOTPExigePour(u *User) bool {
	return s.ModeTOTP() == TOTPObligatoire && u.Peut("chef")
}

// SecretTOTP lit le secret et son état d'activation.
func (s *Store) SecretTOTP(userID int64) (secret string, actif bool, dernierPas int64, err error) {
	var sec sql.NullString
	err = s.DB.QueryRow(`SELECT totp_secret, totp_actif, totp_dernier_pas FROM users WHERE id = ?`,
		userID).Scan(&sec, &actif, &dernierPas)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, 0, ErrNotFound
	}
	return ns(sec), actif, dernierPas, err
}

// PreparerTOTP enregistre un secret sans l'activer : le compte reste
// accessible tant que le premier code n'a pas été saisi avec succès.
func (s *Store) PreparerTOTP(userID int64, secret string) error {
	_, err := s.DB.Exec(`UPDATE users SET totp_secret = ?, totp_actif = 0,
		totp_confirme_at = NULL, totp_dernier_pas = 0 WHERE id = ?`, secret, userID)
	return err
}

// ActiverTOTP met le second facteur en service et remplace les codes de
// secours. Les anciens sont supprimés : ils correspondaient à un secret qui
// n'a plus cours.
func (s *Store) ActiverTOTP(userID int64, pas int64) ([]string, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE users SET totp_actif = 1,
		totp_confirme_at = strftime('%Y-%m-%dT%H:%M:%SZ','now'), totp_dernier_pas = ?
		WHERE id = ?`, pas, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM codes_secours WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}

	codes := make([]string, 0, NombreCodesSecours)
	for i := 0; i < NombreCodesSecours; i++ {
		code, err := nouveauCodeSecours()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO codes_secours (user_id, empreinte) VALUES (?,?)`,
			userID, empreinteCode(code)); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}

// DesactiverTOTP retire le second facteur et ses codes de secours.
func (s *Store) DesactiverTOTP(userID int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET totp_secret = NULL, totp_actif = 0,
		totp_confirme_at = NULL, totp_dernier_pas = 0 WHERE id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM codes_secours WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// MarquerPasTOTP mémorise le dernier pas accepté, ce qui interdit de rejouer
// le même code pendant sa fenêtre de validité.
func (s *Store) MarquerPasTOTP(userID, pas int64) error {
	_, err := s.DB.Exec(`UPDATE users SET totp_dernier_pas = ? WHERE id = ?`, pas, userID)
	return err
}

// --- Codes de secours ---

const NombreCodesSecours = 10

// nouveauCodeSecours produit un code lisible à voix haute et sans ambiguïté
// visuelle : ni 0/O, ni 1/I/l, puisqu'il sera imprimé puis recopié à la main.
func nouveauCodeSecours() (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 0, 11)
	for i, v := range b {
		if i == 5 {
			out = append(out, '-') // deux groupes de cinq, plus faciles à saisir
		}
		out = append(out, alphabet[int(v)%len(alphabet)])
	}
	return string(out), nil
}

func empreinteCode(code string) string {
	somme := sha256.Sum256([]byte(normaliserCode(code)))
	return hex.EncodeToString(somme[:])
}

func normaliserCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
}

// ConsommerCodeSecours vérifie un code et le marque comme utilisé. Un code de
// secours ne sert qu'une fois : c'est ce qui le rend acceptable comme
// substitut au téléphone.
func (s *Store) ConsommerCodeSecours(userID int64, code string) error {
	empreinte := empreinteCode(code)
	res, err := s.DB.Exec(`UPDATE codes_secours
		SET utilise_at = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE user_id = ? AND empreinte = ? AND utilise_at IS NULL`, userID, empreinte)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTOTPInvalide
	}
	return nil
}

func (s *Store) CodesSecoursRestants(userID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM codes_secours
		WHERE user_id = ? AND utilise_at IS NULL`, userID).Scan(&n)
	return n, err
}

// CompterAdminsAvecTOTP sert au garde-fou qui empêche d'activer le mode
// obligatoire depuis un compte qui n'a pas lui-même configuré son second
// facteur : l'administrateur se verrouillerait dehors dans la seconde.
func (s *Store) CompterAdminsAvecTOTP() (avec, total int, err error) {
	err = s.DB.QueryRow(`SELECT
		COALESCE(SUM(totp_actif), 0), COUNT(*)
		FROM users WHERE actif = 1 AND role IN ('chef','admin')`).Scan(&avec, &total)
	return
}

// DescriptionMode rend le mode lisible dans le journal d'audit.
func DescriptionMode(mode string) string {
	switch mode {
	case TOTPFacultatif:
		return "facultatif"
	case TOTPObligatoire:
		return "obligatoire pour les chefs et administrateurs"
	default:
		return "désactivé"
	}
}
