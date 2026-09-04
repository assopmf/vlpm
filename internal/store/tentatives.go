package store

import (
	"strings"
	"time"
)

// Comptage des tentatives de connexion échouées.
//
// Deux limites, appliquées ensemble :
//
//   - par adresse IP, contre un balayage d'identifiants depuis une machine ;
//   - par matricule, contre une attaque du même compte depuis plusieurs
//     adresses, que la seule limite par IP ne voyait pas passer.
//
// Le blocage par matricule se lève tout seul au bout de la fenêtre. C'est un
// compromis assumé : quelqu'un qui connaît le matricule d'un agent peut le
// gêner pendant un quart d'heure en saisissant de mauvais mots de passe. Un
// verrouillage définitif serait pire — il suffirait de viser tous les agents
// d'un service un dimanche soir pour empêcher la prise de service du lundi.
const (
	FenetreTentatives = 15 * time.Minute
	MaxParIP          = 10
	MaxParMatricule   = 5
)

// EnregistrerEchec consigne une tentative infructueuse. Le matricule est
// enregistré qu'il existe ou non : la présence d'une ligne ne doit rien
// apprendre sur l'existence du compte.
func (s *Store) EnregistrerEchec(matricule, ip string) error {
	_, err := s.DB.Exec(`INSERT INTO tentatives_connexion (matricule, ip) VALUES (?,?)`,
		normaliserMatricule(matricule), ip)
	return err
}

// Blocage décrit pourquoi une connexion est refusée avant même d'examiner le
// mot de passe.
type Blocage struct {
	Bloque  bool
	Cause   string        // "ip" ou "matricule"
	Restant time.Duration // délai avant la prochaine tentative possible
}

// ConnexionBloquee indique si les tentatives récentes dépassent une limite.
func (s *Store) ConnexionBloquee(matricule, ip string) (Blocage, error) {
	depuis := time.Now().UTC().Add(-FenetreTentatives).Format(time.RFC3339)

	verifier := func(colonne, valeur string, max int) (Blocage, error) {
		if valeur == "" {
			return Blocage{}, nil
		}
		var n int
		var plusAncienne string
		err := s.DB.QueryRow(
			`SELECT COUNT(*), COALESCE(MIN(created_at), '') FROM tentatives_connexion
			 WHERE `+colonne+` = ? AND created_at >= ?`, valeur, depuis).
			Scan(&n, &plusAncienne)
		if err != nil {
			return Blocage{}, err
		}
		if n < max {
			return Blocage{}, nil
		}
		// Le blocage court jusqu'à ce que la plus ancienne tentative sorte de
		// la fenêtre glissante.
		restant := FenetreTentatives
		if t, err := time.Parse(time.RFC3339, plusAncienne); err == nil {
			restant = time.Until(t.Add(FenetreTentatives))
			if restant < 0 {
				restant = 0
			}
		}
		return Blocage{Bloque: true, Cause: colonne, Restant: restant}, nil
	}

	if b, err := verifier("ip", ip, MaxParIP); err != nil || b.Bloque {
		return b, err
	}
	return verifier("matricule", normaliserMatricule(matricule), MaxParMatricule)
}

// EffacerEchecs solde le compteur après une connexion réussie : un agent qui
// se trompe trois fois puis réussit ne doit pas rester à trois.
func (s *Store) EffacerEchecs(matricule, ip string) error {
	_, err := s.DB.Exec(`DELETE FROM tentatives_connexion WHERE matricule = ? OR ip = ?`,
		normaliserMatricule(matricule), ip)
	return err
}

// PurgerTentatives supprime les tentatives sorties de la fenêtre. Appelée
// périodiquement pour que la table ne grossisse pas indéfiniment.
func (s *Store) PurgerTentatives() (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM tentatives_connexion WHERE created_at < ?`,
		time.Now().UTC().Add(-FenetreTentatives).Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// normaliserMatricule aligne la casse sur celle utilisée à la connexion, pour
// que « Admin » et « admin » comptent bien pour le même compte.
func normaliserMatricule(m string) string {
	return strings.ToLower(strings.TrimSpace(m))
}
