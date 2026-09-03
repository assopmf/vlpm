package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Photo struct {
	ID         int64  `json:"id"`
	IncidentID int64  `json:"incident_id"`
	Fichier    string `json:"-"` // jamais exposé : l'accès passe par l'identifiant
	TypeMIME   string `json:"type_mime"`
	Octets     int64  `json:"octets"`
	Largeur    int    `json:"largeur"`
	Hauteur    int    `json:"hauteur"`
	CreatedAt  string `json:"created_at"`
	AjouteePar string `json:"ajoutee_par,omitempty"`
}

// NomFichierPhoto tire un nom opaque. Le nom fourni par l'appareil n'est jamais
// réutilisé : il pourrait contenir « ../ », des caractères propres à un système
// de fichiers, ou renseigner sur le contenu.
func NomFichierPhoto(extension string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + extension, nil
}

func (s *Store) CreatePhoto(p *Photo, userID int64) error {
	var par any
	if userID > 0 {
		par = userID
	}
	res, err := s.DB.Exec(`INSERT INTO photos
		(incident_id, fichier, type_mime, octets, largeur, hauteur, ajoutee_par)
		VALUES (?,?,?,?,?,?,?)`,
		p.IncidentID, p.Fichier, p.TypeMIME, p.Octets, p.Largeur, p.Hauteur, par)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	// L'horodatage vient d'une valeur par défaut SQL : sans relecture, la
	// réponse renvoyée au client porterait une date vide.
	return s.DB.QueryRow(`SELECT created_at FROM photos WHERE id = ?`, p.ID).Scan(&p.CreatedAt)
}

func (s *Store) PhotosDIncident(incidentID int64) ([]Photo, error) {
	rows, err := s.DB.Query(`SELECT p.id, p.incident_id, p.fichier, p.type_mime, p.octets,
		p.largeur, p.hauteur, p.created_at, COALESCE(u.prenom || ' ' || u.nom, '')
		FROM photos p LEFT JOIN users u ON u.id = p.ajoutee_par
		WHERE p.incident_id = ? ORDER BY p.id`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.IncidentID, &p.Fichier, &p.TypeMIME, &p.Octets,
			&p.Largeur, &p.Hauteur, &p.CreatedAt, &p.AjouteePar); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PhotoByID renvoie une photo et le véhicule dont elle dépend, pour que
// l'appelant puisse contrôler les droits avant de servir le fichier.
func (s *Store) PhotoByID(id int64) (*Photo, int64, error) {
	var p Photo
	var vehicleID int64
	err := s.DB.QueryRow(`SELECT p.id, p.incident_id, p.fichier, p.type_mime, p.octets,
		p.largeur, p.hauteur, p.created_at, i.vehicle_id
		FROM photos p JOIN incidents i ON i.id = p.incident_id WHERE p.id = ?`, id).
		Scan(&p.ID, &p.IncidentID, &p.Fichier, &p.TypeMIME, &p.Octets,
			&p.Largeur, &p.Hauteur, &p.CreatedAt, &vehicleID)
	if err == sql.ErrNoRows {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return &p, vehicleID, nil
}

func (s *Store) SupprimerPhoto(id int64) (string, error) {
	var fichier string
	err := s.DB.QueryRow(`SELECT fichier FROM photos WHERE id = ?`, id).Scan(&fichier)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := s.DB.Exec(`DELETE FROM photos WHERE id = ?`, id); err != nil {
		return "", err
	}
	return fichier, nil
}

// BalayerPhotosOrphelines supprime du disque les fichiers dont plus aucune
// ligne ne parle.
//
// C'est le filet de sécurité de tout le mécanisme : la suppression d'un
// incident, d'un véhicule ou d'une purge RGPD efface des lignes en cascade,
// sans que le code appelant ait à penser aux fichiers. Sans ce balayage, une
// purge de conservation laisserait sur le disque des photos que la commune
// s'est engagée à supprimer.
func BalayerPhotosOrphelines(st *Store, dossier string) (int, error) {
	entrees, err := os.ReadDir(dossier)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	connus := map[string]bool{}
	rows, err := st.DB.Query(`SELECT fichier FROM photos`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return 0, err
		}
		connus[f] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	supprimees := 0
	for _, e := range entrees {
		if e.IsDir() || connus[e.Name()] {
			continue
		}
		// Ne touche qu'aux fichiers que nous produisons : 32 caractères
		// hexadécimaux suivis d'une extension d'image.
		if !nomDePhoto(e.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(dossier, e.Name())); err != nil {
			return supprimees, err
		}
		supprimees++
	}
	return supprimees, nil
}

func nomDePhoto(nom string) bool {
	ext := filepath.Ext(nom)
	switch ext {
	case ".jpg", ".png", ".webp":
	default:
		return false
	}
	base := strings.TrimSuffix(nom, ext)
	if len(base) != 32 {
		return false
	}
	_, err := hex.DecodeString(base)
	return err == nil
}

// CheminPhoto compose le chemin d'un fichier en refusant tout nom qui ne
// provient pas de notre générateur. Le nom vient de la base, mais une
// vérification de plus coûte moins cher qu'une traversée de répertoire.
func CheminPhoto(dossier, fichier string) (string, error) {
	if !nomDePhoto(fichier) {
		return "", fmt.Errorf("nom de fichier invalide : %q", fichier)
	}
	return filepath.Join(dossier, fichier), nil
}

// photosParIncident charge toutes les photos en une requête, pour éviter un
// aller-retour par incident lors de l'affichage d'une liste.
func (s *Store) photosParIncident() (map[int64][]Photo, error) {
	rows, err := s.DB.Query(`SELECT p.id, p.incident_id, p.fichier, p.type_mime, p.octets,
		p.largeur, p.hauteur, p.created_at, COALESCE(u.prenom || ' ' || u.nom, '')
		FROM photos p LEFT JOIN users u ON u.id = p.ajoutee_par
		ORDER BY p.incident_id, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := map[int64][]Photo{}
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.IncidentID, &p.Fichier, &p.TypeMIME, &p.Octets,
			&p.Largeur, &p.Hauteur, &p.CreatedAt, &p.AjouteePar); err != nil {
			return nil, err
		}
		m[p.IncidentID] = append(m[p.IncidentID], p)
	}
	return m, rows.Err()
}
