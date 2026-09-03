package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/assopmf/vlpm/internal/courriel"
	"github.com/assopmf/vlpm/internal/store"
)

// getNotifications renvoie les réglages d'envoi, l'état du relais SMTP et la
// liste effective des destinataires.
func (s *Server) getNotifications(w http.ResponseWriter, r *http.Request) {
	n := s.st.Notifications()
	effectifs, err := s.st.DestinatairesEffectifs()
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"notifications":           n,
		"destinataires_effectifs": courriel.AdressesValides(effectifs),
		// Sans relais configuré, l'interface doit expliquer pourquoi le
		// réglage ne produira aucun effet, plutôt que de laisser croire
		// qu'il fonctionne.
		"envoi_configure": s.courriel != nil && s.cfg.SMTPHote != "",
		"expediteur":      s.cfg.SMTPExpediteur,
		"prochain_envoi":  s.st.ProchainEnvoi(),
	})
}

type requeteNotifications struct {
	Frequence     *string   `json:"frequence"`
	Destinataires *[]string `json:"destinataires"`
}

func (s *Server) patchNotifications(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteNotifications
	if !decoderJSON(w, r, &req) {
		return
	}
	n := s.st.Notifications()
	if req.Frequence != nil {
		n.Frequence = *req.Frequence
	}
	if req.Destinataires != nil {
		valides := courriel.AdressesValides(*req.Destinataires)
		// Une adresse mal saisie doit être signalée, pas ignorée en silence :
		// sinon un chef croit être destinataire et ne reçoit jamais rien.
		if len(valides) != len(nonVides(*req.Destinataires)) {
			erreur(w, http.StatusUnprocessableEntity,
				"Une des adresses saisies n'est pas une adresse électronique valide.",
				"adresse_invalide")
			return
		}
		n.Destinataires = valides
	}
	if err := s.st.DefinirNotifications(n); err != nil {
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "frequence_invalide")
		return
	}
	s.st.Audit(u.ID, "notifications_modifiees", "settings", 0, n.Frequence, s.ipDe(r))
	s.getNotifications(w, r)
}

// postEnvoyerReleve expédie un relevé sur-le-champ. Sert à vérifier la
// configuration du relais sans attendre l'échéance.
func (s *Server) postEnvoyerReleve(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	if s.courriel == nil {
		erreur(w, http.StatusUnprocessableEntity,
			"Aucun serveur d'envoi n'est configuré sur ce serveur. "+
				"Renseignez --smtp-hote et --smtp-expediteur au démarrage.",
			"smtp_absent")
		return
	}
	destinataires, err := s.st.DestinatairesEffectifs()
	if err != nil {
		erreurStore(w, err)
		return
	}
	if len(courriel.AdressesValides(destinataires)) == 0 {
		erreur(w, http.StatusUnprocessableEntity,
			"Aucun destinataire : ajoutez une adresse, ou renseignez le courriel des chefs de service.",
			"sans_destinataire")
		return
	}

	alertes, err := s.st.Alertes()
	if err != nil {
		erreurStore(w, err)
		return
	}
	if len(alertes) == 0 {
		erreur(w, http.StatusUnprocessableEntity,
			"Aucune échéance à signaler : il n'y a rien à envoyer.", "rien_a_signaler")
		return
	}

	sujet, corps := store.ComposerReleve(s.cfg.NomService, alertes, s.cfg.BaseURL)
	if err := s.courriel.Envoyer(courriel.Message{
		Destinataires: destinataires, Sujet: sujet, Corps: corps,
	}); err != nil {
		s.log.Error("envoi du relevé", "erreur", err)
		erreur(w, http.StatusBadGateway,
			"L'envoi a échoué : "+err.Error(), "envoi_echoue")
		return
	}
	if err := s.st.MarquerEnvoi(time.Now()); err != nil {
		s.log.Warn("horodatage de l'envoi", "erreur", err)
	}
	s.st.Audit(u.ID, "releve_envoye", "settings", 0,
		majuscule(sujet), s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]any{
		"message":        "Relevé envoyé.",
		"destinataires":  courriel.AdressesValides(destinataires),
		"prochain_envoi": s.st.ProchainEnvoi(),
	})
}

// nonVides écarte les lignes blanches d'une saisie libre, pour ne pas compter
// une ligne vide comme une adresse invalide.
func nonVides(liste []string) []string {
	out := []string{}
	for _, v := range liste {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
