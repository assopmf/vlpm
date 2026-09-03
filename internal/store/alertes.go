package store

import (
	"database/sql"
	"time"
)

// Alertes d'échéance.
//
// Les dates de contrôle technique et les seuils de révision étaient enregistrés
// et affichés, mais rien ne les signalait à l'avance : l'échéance se découvrait
// le jour où le véhicule devenait inutilisable.

const (
	// PreavisCTJours : un contrôle technique se prend rarement du jour au
	// lendemain, et un mois laisse le temps de caler un rendez-vous.
	PreavisCTJours = 30
	// PreavisRevisionKM : marge avant le seuil de révision saisi.
	PreavisRevisionKM = 1000
)

type Alerte struct {
	VehiculeID    int64  `json:"vehicule_id"`
	Code          string `json:"code"`
	Modele        string `json:"modele"`
	Type          string `json:"type"`    // "controle_technique" ou "revision"
	Gravite       string `json:"gravite"` // "depassee" ou "proche"
	Echeance      string `json:"echeance,omitempty"`
	JoursRestants *int   `json:"jours_restants,omitempty"`
	KMRestants    *int64 `json:"km_restants,omitempty"`
	Message       string `json:"message"`
}

// Alertes recense les échéances dépassées ou proches sur le parc actif.
// Les véhicules archivés en sont exclus : ils ne roulent plus.
func (s *Store) Alertes() ([]Alerte, error) {
	rows, err := s.DB.Query(`SELECT id, code, marque, modele, km, prochain_ct, prochaine_revision_km
		FROM vehicles WHERE archive = 0 ORDER BY code COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aujourdhui := time.Now().Truncate(24 * time.Hour)
	out := []Alerte{}

	for rows.Next() {
		var id, km int64
		var code, marque, modele string
		var ct sql.NullString
		var revKM sql.NullInt64
		if err := rows.Scan(&id, &code, &marque, &modele, &km, &ct, &revKM); err != nil {
			return nil, err
		}
		libelle := marque
		if modele != "" {
			libelle = marque + " " + modele
		}

		if ct.Valid && ct.String != "" {
			if a, ok := alerteCT(id, code, libelle, ct.String, aujourdhui); ok {
				out = append(out, a)
			}
		}
		if revKM.Valid && revKM.Int64 > 0 {
			if a, ok := alerteRevision(id, code, libelle, km, revKM.Int64); ok {
				out = append(out, a)
			}
		}
	}
	return out, rows.Err()
}

func alerteCT(id int64, code, modele, echeance string, aujourdhui time.Time) (Alerte, bool) {
	date, err := time.Parse("2006-01-02", echeance[:min(10, len(echeance))])
	if err != nil {
		return Alerte{}, false // date illisible : on n'invente pas d'alerte
	}
	jours := int(date.Sub(aujourdhui).Hours() / 24)
	if jours > PreavisCTJours {
		return Alerte{}, false
	}

	a := Alerte{
		VehiculeID: id, Code: code, Modele: modele,
		Type: "controle_technique", Echeance: echeance, JoursRestants: &jours,
	}
	switch {
	case jours < 0:
		a.Gravite = "depassee"
		a.Message = "Contrôle technique dépassé depuis " + jourss(-jours)
	case jours == 0:
		a.Gravite = "depassee"
		a.Message = "Contrôle technique à faire aujourd'hui"
	default:
		a.Gravite = "proche"
		a.Message = "Contrôle technique dans " + jourss(jours)
	}
	return a, true
}

func alerteRevision(id int64, code, modele string, km, seuil int64) (Alerte, bool) {
	restants := seuil - km
	if restants > PreavisRevisionKM {
		return Alerte{}, false
	}

	a := Alerte{
		VehiculeID: id, Code: code, Modele: modele,
		Type: "revision", KMRestants: &restants,
	}
	if restants <= 0 {
		a.Gravite = "depassee"
		a.Message = "Révision dépassée de " + FmtKM(-restants) + " km"
	} else {
		a.Gravite = "proche"
		a.Message = "Révision dans " + FmtKM(restants) + " km"
	}
	return a, true
}

func jourss(n int) string {
	if n <= 1 {
		return "1 jour"
	}
	return FmtKM(int64(n)) + " jours"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
