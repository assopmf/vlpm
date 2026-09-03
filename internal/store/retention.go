package store

import (
	"fmt"
	"strconv"
	"time"
)

// Conservation des données.
//
// Le RGPD impose une durée de conservation définie et justifiée. Elle relève
// du responsable de traitement — le DPO de la commune — et non du code : elle
// se règle donc depuis l'application, sans redéploiement.
//
// Par défaut, rien n'est supprimé. Une instance qui effacerait des données
// sans que personne l'ait décidé serait un défaut, pas une fonctionnalité.

const (
	CleConservationActivite = "conservation_activite_mois"
	CleConservationJournal  = "conservation_journal_mois"
	CleDernierePurge        = "derniere_purge"
)

type Conservation struct {
	// ActiviteMois : sorties terminées et incidents résolus. 0 = illimité.
	ActiviteMois int `json:"activite_mois"`
	// JournalMois : journal d'audit. 0 = illimité.
	JournalMois int `json:"journal_mois"`
	// DernierePurge : horodatage de la dernière exécution, vide si jamais.
	DernierePurge string `json:"derniere_purge,omitempty"`
}

// BilanPurge dénombre les enregistrements concernés, que ce soit pour un
// aperçu avant activation ou pour rendre compte d'une purge effectuée.
type BilanPurge struct {
	Sorties   int `json:"sorties"`
	Incidents int `json:"incidents"`
	Journal   int `json:"journal"`
}

func (b BilanPurge) Total() int { return b.Sorties + b.Incidents + b.Journal }

func (s *Store) Conservation() Conservation {
	return Conservation{
		ActiviteMois:  s.settingInt(CleConservationActivite, 0),
		JournalMois:   s.settingInt(CleConservationJournal, 0),
		DernierePurge: s.Setting(CleDernierePurge, ""),
	}
}

func (s *Store) DefinirConservation(c Conservation) error {
	if c.ActiviteMois < 0 || c.JournalMois < 0 {
		return fmt.Errorf("une durée de conservation ne peut pas être négative")
	}
	// Un an de recul minimum quand la purge est active : en dessous, on
	// effacerait des données de l'exercice budgétaire en cours.
	if c.ActiviteMois > 0 && c.ActiviteMois < 12 {
		return fmt.Errorf("la durée de conservation de l'activité doit être d'au moins 12 mois")
	}
	if c.JournalMois > 0 && c.JournalMois < 6 {
		return fmt.Errorf("la durée de conservation du journal doit être d'au moins 6 mois")
	}
	if err := s.SetSetting(CleConservationActivite, strconv.Itoa(c.ActiviteMois)); err != nil {
		return err
	}
	return s.SetSetting(CleConservationJournal, strconv.Itoa(c.JournalMois))
}

func (s *Store) settingInt(cle string, def int) int {
	if n, err := strconv.Atoi(s.Setting(cle, "")); err == nil {
		return n
	}
	return def
}

// limite compose une date au format exact des colonnes, faute de quoi la
// comparaison de chaînes serait fausse : strftime('%Y-%m-%d %H:%M:%S') produit
// un espace là où le schéma stocke un T, et pas de Z final.
func limite(mois int) string {
	return time.Now().UTC().AddDate(0, -mois, 0).Format(time.RFC3339)
}

// SimulerPurge dénombre ce qu'une purge supprimerait, sans rien effacer.
// Indispensable avant d'activer une durée : l'opération est irréversible.
func (s *Store) SimulerPurge(c Conservation) (BilanPurge, error) {
	var b BilanPurge

	if c.ActiviteMois > 0 {
		avant := limite(c.ActiviteMois)
		// Les sorties en cours ne sont jamais concernées, quelle que soit leur
		// ancienneté : une sortie ouverte depuis deux ans est une anomalie à
		// traiter, pas une donnée à effacer.
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM checkouts
			WHERE statut = 'termine' AND ended_at < ?`, avant).Scan(&b.Sorties); err != nil {
			return b, err
		}
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM incidents
			WHERE statut = 'resolu' AND resolu_at < ?`, avant).Scan(&b.Incidents); err != nil {
			return b, err
		}
	}
	if c.JournalMois > 0 {
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE created_at < ?`,
			limite(c.JournalMois)).Scan(&b.Journal); err != nil {
			return b, err
		}
	}
	return b, nil
}

// Purger supprime définitivement les données au-delà des durées configurées.
func (s *Store) Purger(c Conservation) (BilanPurge, error) {
	var b BilanPurge
	tx, err := s.DB.Begin()
	if err != nil {
		return b, err
	}
	defer tx.Rollback()

	if c.ActiviteMois > 0 {
		avant := limite(c.ActiviteMois)
		res, err := tx.Exec(`DELETE FROM checkouts WHERE statut = 'termine' AND ended_at < ?`, avant)
		if err != nil {
			return b, err
		}
		n, _ := res.RowsAffected()
		b.Sorties = int(n)

		res, err = tx.Exec(`DELETE FROM incidents WHERE statut = 'resolu' AND resolu_at < ?`, avant)
		if err != nil {
			return b, err
		}
		n, _ = res.RowsAffected()
		b.Incidents = int(n)
	}

	if c.JournalMois > 0 {
		res, err := tx.Exec(`DELETE FROM audit_log WHERE created_at < ?`, limite(c.JournalMois))
		if err != nil {
			return b, err
		}
		n, _ := res.RowsAffected()
		b.Journal = int(n)
	}

	if err := tx.Commit(); err != nil {
		return b, err
	}
	if b.Total() > 0 {
		_ = s.SetSetting(CleDernierePurge, time.Now().UTC().Format(time.RFC3339))
	}
	return b, nil
}
