package api

import (
	"net/http"
	"strconv"

	"github.com/assopmf/vlpm/internal/store"
)

// getConservation renvoie les durées configurées, accompagnées d'un aperçu de
// ce qu'une purge supprimerait aujourd'hui.
func (s *Server) getConservation(w http.ResponseWriter, r *http.Request) {
	c := s.st.Conservation()
	apercu, err := s.st.SimulerPurge(c)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"conservation": c,
		"a_purger":     apercu,
	})
}

type requeteConservation struct {
	ActiviteMois *int `json:"activite_mois"`
	JournalMois  *int `json:"journal_mois"`
}

func (s *Server) patchConservation(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteConservation
	if !decoderJSON(w, r, &req) {
		return
	}
	c := s.st.Conservation()
	if req.ActiviteMois != nil {
		c.ActiviteMois = *req.ActiviteMois
	}
	if req.JournalMois != nil {
		c.JournalMois = *req.JournalMois
	}
	if err := s.st.DefinirConservation(c); err != nil {
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "duree_invalide")
		return
	}
	s.st.Audit(u.ID, "conservation_modifiee", "settings", 0,
		formatDurees(c), s.ipDe(r))
	s.getConservation(w, r)
}

// postSimulerPurge chiffre l'effet d'une durée avant qu'elle soit enregistrée.
// Une purge étant irréversible, l'administrateur doit voir le nombre exact
// d'enregistrements concernés avant de valider.
func (s *Server) postSimulerPurge(w http.ResponseWriter, r *http.Request) {
	var req requeteConservation
	if !decoderJSON(w, r, &req) {
		return
	}
	c := s.st.Conservation()
	if req.ActiviteMois != nil {
		c.ActiviteMois = *req.ActiviteMois
	}
	if req.JournalMois != nil {
		c.JournalMois = *req.JournalMois
	}
	apercu, err := s.st.SimulerPurge(c)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{"a_purger": apercu})
}

// postPurger déclenche la purge immédiatement, sans attendre l'échéance
// quotidienne.
func (s *Server) postPurger(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	c := s.st.Conservation()
	if c.ActiviteMois == 0 && c.JournalMois == 0 {
		erreur(w, http.StatusUnprocessableEntity,
			"Aucune durée de conservation n'est définie : il n'y a rien à purger.",
			"conservation_illimitee")
		return
	}
	bilan, err := s.st.Purger(c)
	if err != nil {
		erreurStore(w, err)
		return
	}
	// La purge efface le journal d'audit : cette entrée est écrite après, pour
	// qu'elle survive à l'opération qu'elle relate.
	s.st.Audit(u.ID, "purge_manuelle", "settings", 0, formatBilan(bilan), s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]any{
		"supprime":     bilan,
		"conservation": s.st.Conservation(),
	})
}

func formatDurees(c store.Conservation) string {
	return "activité=" + moisOuIllimite(c.ActiviteMois) + " journal=" + moisOuIllimite(c.JournalMois)
}

func moisOuIllimite(mois int) string {
	if mois == 0 {
		return "illimité"
	}
	return strconv.Itoa(mois) + " mois"
}

func formatBilan(b store.BilanPurge) string {
	return "sorties=" + strconv.Itoa(b.Sorties) +
		" incidents=" + strconv.Itoa(b.Incidents) +
		" journal=" + strconv.Itoa(b.Journal)
}
