package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Session décrit un appareil connecté. Le jeton lui-même n'apparaît jamais :
// seul son identifiant de ligne sert à désigner la session à révoquer.
type Session struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Appareil  string `json:"appareil"`
	IP        string `json:"ip"`
	Actuelle  bool   `json:"actuelle"`
}

// SessionsDeLUtilisateur liste les sessions encore valides. L'agent y
// reconnaît ses appareils, et peut couper celui qu'il a perdu sans attendre
// qu'un chef désactive tout son compte.
func (s *Store) SessionsDeLUtilisateur(userID int64, jetonActuel string) ([]Session, error) {
	rows, err := s.DB.Query(`SELECT rowid, token, created_at, expires_at, user_agent, ip
		FROM sessions WHERE user_id = ? AND expires_at >= ?
		ORDER BY created_at DESC`,
		userID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Session{}
	for rows.Next() {
		var s Session
		var token, agent string
		if err := rows.Scan(&s.ID, &token, &s.CreatedAt, &s.ExpiresAt, &agent, &s.IP); err != nil {
			return nil, err
		}
		s.Appareil = DecrireAppareil(agent)
		s.Actuelle = token == jetonActuel
		out = append(out, s)
	}
	return out, rows.Err()
}

// RevoquerSession coupe une session précise. Le filtre sur user_id est
// essentiel : sans lui, un agent pourrait déconnecter n'importe qui en
// devinant un identifiant de ligne.
func (s *Store) RevoquerSession(userID, sessionID int64) error {
	res, err := s.DB.Exec(`DELETE FROM sessions WHERE rowid = ? AND user_id = ?`,
		sessionID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RevoquerAutresSessions coupe tout sauf l'appareil courant. C'est le geste à
// faire quand on ne sait plus lesquels sont légitimes.
func (s *Store) RevoquerAutresSessions(userID int64, jetonActuel string) (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM sessions WHERE user_id = ? AND token != ?`,
		userID, jetonActuel)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SessionParID sert à vérifier l'appartenance avant d'agir.
func (s *Store) SessionParID(sessionID int64) (int64, error) {
	var userID int64
	err := s.DB.QueryRow(`SELECT user_id FROM sessions WHERE rowid = ?`, sessionID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return userID, err
}

// DecrireAppareil traduit un en-tête User-Agent en une mention lisible.
// L'objectif n'est pas d'identifier précisément le matériel, mais de permettre
// à un agent de reconnaître son propre téléphone dans une liste.
func DecrireAppareil(agent string) string {
	if agent == "" {
		return "Appareil inconnu"
	}
	systeme := ""
	switch {
	case strings.Contains(agent, "Android"):
		systeme = "Android"
	case strings.Contains(agent, "iPhone"):
		systeme = "iPhone"
	case strings.Contains(agent, "iPad"):
		systeme = "iPad"
	case strings.Contains(agent, "Windows"):
		systeme = "Windows"
	case strings.Contains(agent, "Macintosh"), strings.Contains(agent, "Mac OS"):
		systeme = "Mac"
	case strings.Contains(agent, "Linux"):
		systeme = "Linux"
	}

	navigateur := ""
	switch {
	// L'ordre compte : Chrome et Edge annoncent aussi « Safari », et Edge
	// annonce « Chrome ». Le plus spécifique doit être testé en premier.
	case strings.Contains(agent, "VLPM"):
		navigateur = "application VLPM"
	case strings.Contains(agent, "Edg/"):
		navigateur = "Edge"
	case strings.Contains(agent, "Firefox"):
		navigateur = "Firefox"
	case strings.Contains(agent, "Chrome"):
		navigateur = "Chrome"
	case strings.Contains(agent, "Safari"):
		navigateur = "Safari"
	}

	switch {
	case systeme != "" && navigateur != "":
		return systeme + " — " + navigateur
	case systeme != "":
		return systeme
	case navigateur != "":
		return navigateur
	default:
		return "Appareil inconnu"
	}
}
