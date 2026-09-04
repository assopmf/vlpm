package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/assopmf/vlpm/internal/store"
	"github.com/assopmf/vlpm/internal/totp"
)

// getTOTP décrit l'état du second facteur pour le compte courant.
func (s *Server) getTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	_, actif, _, err := s.st.SecretTOTP(u.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	restants, err := s.st.CodesSecoursRestants(u.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"actif":                  actif,
		"mode":                   s.st.ModeTOTP(),
		"exige":                  s.st.TOTPExigePour(u),
		"codes_secours_restants": restants,
	})
}

// postPreparerTOTP tire un secret et renvoie de quoi l'inscrire dans une
// application. Le second facteur n'est pas encore actif : le compte reste
// accessible tant que le premier code n'a pas été validé.
func (s *Server) postPreparerTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	secret, err := totp.NouveauSecret()
	if err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.st.PreparerTOTP(u.ID, secret); err != nil {
		erreurStore(w, err)
		return
	}
	emetteur := s.cfg.NomService
	if emetteur == "" {
		emetteur = "VLPM"
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"uri": totp.URI(secret, emetteur, u.Matricule),
		// Le secret en clair permet la saisie manuelle, quand la caméra ne
		// peut pas lire le QR code.
		"secret": secret,
	})
}

// getQRCodeTOTP rend le QR d'inscription. Le jeton de session suffit : le
// secret n'est lisible que par le titulaire du compte.
func (s *Server) getQRCodeTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	secret, actif, _, err := s.st.SecretTOTP(u.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if secret == "" || actif {
		// Une fois le second facteur en service, le QR n'a plus à être servi :
		// il permettrait d'inscrire un appareil de plus sans rien prouver.
		erreur(w, http.StatusConflict,
			"Aucune inscription en cours. Lancez la configuration pour obtenir un nouveau code.",
			"pas_de_preparation")
		return
	}
	png, err := qrcode.Encode(totp.URI(secret, nomOuDefaut(s.cfg.NomService), u.Matricule),
		qrcode.Medium, 320)
	if err != nil {
		erreurStore(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

type requeteCodeTOTP struct {
	Code       string `json:"code"`
	MotDePasse string `json:"mot_de_passe"`
}

// postActiverTOTP met le second facteur en service après vérification d'un
// premier code, ce qui prouve que l'application est correctement inscrite.
func (s *Server) postActiverTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteCodeTOTP
	if !decoderJSON(w, r, &req) {
		return
	}
	secret, actif, _, err := s.st.SecretTOTP(u.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if actif {
		erreur(w, http.StatusConflict,
			"Le second facteur est déjà en service sur ce compte.", "deja_actif")
		return
	}
	if secret == "" {
		erreur(w, http.StatusConflict,
			"Lancez d'abord la configuration pour obtenir un code à scanner.",
			"pas_de_preparation")
		return
	}

	pas, ok := totp.Verifier(secret, req.Code, time.Now())
	if !ok {
		erreur(w, http.StatusUnprocessableEntity,
			"Code incorrect. Vérifiez que l'heure de votre téléphone est à jour.",
			"totp_invalide")
		return
	}

	codes, err := s.st.ActiverTOTP(u.ID, pas)
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "totp_active", "user", u.ID, "", s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]any{
		"message":       "Second facteur activé.",
		"codes_secours": codes,
		"avertissement": "Ces codes ne seront plus affichés. Imprimez-les et " +
			"rangez-les en lieu sûr : ils sont le seul moyen de retrouver l'accès " +
			"si vous perdez votre téléphone.",
	})
}

// deleteTOTP retire le second facteur. Le mot de passe est redemandé : sans
// cela, un jeton volé permettrait de désactiver la protection qu'il contourne.
func (s *Server) deleteTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteCodeTOTP
	if !decoderJSON(w, r, &req) {
		return
	}
	if !s.auth.VerifierMotDePasse(u, req.MotDePasse) {
		erreur(w, http.StatusUnauthorized, "Mot de passe incorrect.", "identifiants")
		return
	}
	if s.st.TOTPExigePour(u) {
		erreur(w, http.StatusConflict,
			"Votre service impose le second facteur pour les chefs et administrateurs. "+
				"Il ne peut pas être retiré.", "totp_obligatoire")
		return
	}
	if err := s.st.DesactiverTOTP(u.ID); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "totp_desactive", "user", u.ID, "", s.ipDe(r))
	w.WriteHeader(http.StatusNoContent)
}

// postReinitialiserTOTPAgent permet à un administrateur de débloquer un agent
// qui a perdu son téléphone et ses codes de secours.
func (s *Server) postReinitialiserTOTPAgent(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	cible, err := s.st.UserByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.st.DesactiverTOTP(cible.ID); err != nil {
		erreurStore(w, err)
		return
	}
	// Les sessions tombent : si le téléphone a été volé plutôt que perdu, il
	// ne doit pas conserver un accès ouvert.
	_ = s.auth.LogoutTous(cible.ID)
	s.st.Audit(acteur.ID, "totp_reinitialise", "user", cible.ID,
		"par "+acteur.NomComplet(), s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf(
			"Second facteur retiré pour %s. Ses sessions ont été fermées ; "+
				"il devra le reconfigurer à sa prochaine connexion si votre service l'impose.",
			cible.NomComplet()),
	})
}

// --- Politique du service ---

type requeteModeTOTP struct {
	Mode string `json:"mode"`
}

func (s *Server) patchModeTOTP(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteModeTOTP
	if !decoderJSON(w, r, &req) {
		return
	}

	// Garde-fou : activer le mode obligatoire depuis un compte dépourvu de
	// second facteur verrouillerait son auteur dehors à la déconnexion
	// suivante. On refuse plutôt que de laisser faire.
	if req.Mode == store.TOTPObligatoire {
		_, actif, _, err := s.st.SecretTOTP(u.ID)
		if err != nil {
			erreurStore(w, err)
			return
		}
		if !actif {
			erreur(w, http.StatusConflict,
				"Configurez d'abord votre propre second facteur : sans lui, "+
					"vous ne pourriez plus vous connecter après ce changement.",
				"totp_auteur_manquant")
			return
		}
	}

	if err := s.st.DefinirModeTOTP(req.Mode); err != nil {
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "mode_invalide")
		return
	}
	avec, total, err := s.st.CompterAdminsAvecTOTP()
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "mode_totp_modifie", "settings", 0,
		store.DescriptionMode(req.Mode), s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]any{
		"mode":                   s.st.ModeTOTP(),
		"responsables_avec_totp": avec,
		"responsables_total":     total,
	})
}

func (s *Server) getModeTOTP(w http.ResponseWriter, r *http.Request) {
	avec, total, err := s.st.CompterAdminsAvecTOTP()
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"mode":                   s.st.ModeTOTP(),
		"responsables_avec_totp": avec,
		"responsables_total":     total,
	})
}

func nomOuDefaut(nom string) string {
	if nom == "" {
		return "VLPM"
	}
	return nom
}
