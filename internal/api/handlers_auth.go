package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/assopmf/vlpm/internal/auth"
	"github.com/assopmf/vlpm/internal/store"
)

type requeteLogin struct {
	Matricule  string `json:"matricule"`
	MotDePasse string `json:"mot_de_passe"`
}

type reponseLogin struct {
	Token      string      `json:"token"`
	ExpireDans int         `json:"expire_dans_secondes"`
	User       *store.User `json:"user"`
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	var req requeteLogin
	if !decoderJSON(w, r, &req) {
		return
	}
	req.Matricule = strings.TrimSpace(req.Matricule)
	if req.Matricule == "" || req.MotDePasse == "" {
		erreur(w, http.StatusBadRequest, "Matricule et mot de passe sont obligatoires.", "champs_manquants")
		return
	}

	ip := s.ipDe(r)

	// Le blocage est vérifié avant toute comparaison de mot de passe, sur
	// l'adresse comme sur le matricule : la seule limite par IP ne voyait pas
	// une attaque du même compte menée depuis plusieurs adresses.
	blocage, err := s.st.ConnexionBloquee(req.Matricule, ip)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if blocage.Bloque {
		s.st.Audit(0, "login_bloque", "user", 0,
			"matricule="+req.Matricule+" cause="+blocage.Cause, ip)
		erreur(w, http.StatusTooManyRequests, fmt.Sprintf(
			"Trop de tentatives de connexion. Réessayez dans %s.",
			delaiLisible(blocage.Restant)), "trop_de_tentatives")
		return
	}

	u, token, err := s.auth.Login(req.Matricule, req.MotDePasse, r.UserAgent(), ip)
	if err != nil {
		if errors.Is(err, auth.ErrCompteInactif) {
			erreur(w, http.StatusForbidden, "Ce compte est désactivé. Contactez votre responsable.", "compte_inactif")
			return
		}
		if errors.Is(err, auth.ErrIdentifiants) {
			if err := s.st.EnregistrerEchec(req.Matricule, ip); err != nil {
				s.log.Warn("enregistrement d'une tentative échouée", "erreur", err)
			}
			s.st.Audit(0, "login_echec", "user", 0, "matricule="+req.Matricule, ip)
			erreur(w, http.StatusUnauthorized, "Matricule ou mot de passe incorrect.", "identifiants")
			return
		}
		erreurStore(w, err)
		return
	}

	if err := s.st.EffacerEchecs(req.Matricule, ip); err != nil {
		s.log.Warn("remise à zéro des tentatives", "erreur", err)
	}
	s.st.Audit(u.ID, "login", "user", u.ID, "", ip)
	ecrireJSON(w, http.StatusOK, reponseLogin{
		Token:      token,
		ExpireDans: int(auth.DureeSession.Seconds()),
		User:       u,
	})
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	if err := s.auth.Logout(jetonDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "logout", "user", u.ID, "", s.ipDe(r))
	w.WriteHeader(http.StatusNoContent)
}

// getMoi renvoie l'utilisateur de la session en cours, avec le véhicule qu'il
// détient éventuellement : l'écran d'accueil s'en sert pour proposer
// directement la restitution.
func (s *Server) getMoi(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	rep := map[string]any{"user": u}
	if c, err := s.st.CheckoutEnCoursPourAgent(u.ID); err == nil {
		rep["checkout_en_cours"] = c
	}
	ecrireJSON(w, http.StatusOK, rep)
}

type requeteMotDePasse struct {
	Actuel  string `json:"mot_de_passe_actuel"`
	Nouveau string `json:"nouveau_mot_de_passe"`
}

func (s *Server) postChangerMotDePasse(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteMotDePasse
	if !decoderJSON(w, r, &req) {
		return
	}
	// On revérifie le mot de passe actuel : le jeton de session ne suffit pas
	// pour une opération qui invalide toutes les autres sessions.
	if !s.auth.VerifierMotDePasse(u, req.Actuel) {
		erreur(w, http.StatusUnauthorized, "Mot de passe actuel incorrect.", "identifiants")
		return
	}
	if err := auth.ValiderMotDePasse(req.Nouveau); err != nil {
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "mot_de_passe_faible")
		return
	}
	hash, err := auth.HashPassword(req.Nouveau)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.st.SetPassword(u.ID, hash, false); err != nil {
		erreurStore(w, err)
		return
	}
	// Toutes les sessions tombent, y compris celle-ci : l'agent se reconnecte.
	_ = s.auth.LogoutTous(u.ID)
	s.st.Audit(u.ID, "changement_mot_de_passe", "user", u.ID, "", s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]string{
		"message": "Mot de passe modifié. Reconnectez-vous avec le nouveau.",
	})
}

// --- Gestion des comptes (réservée aux chefs et admins) ---

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request) {
	inclureInactifs := r.URL.Query().Get("inactifs") == "1"
	users, err := s.st.ListUsers(inclureInactifs)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, users)
}

type requeteUser struct {
	Matricule  string `json:"matricule"`
	Nom        string `json:"nom"`
	Prenom     string `json:"prenom"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Actif      *bool  `json:"actif"`
	MotDePasse string `json:"mot_de_passe"`
}

var rolesValides = map[string]bool{"agent": true, "chef": true, "admin": true}

func (s *Server) postUsers(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	var req requeteUser
	if !decoderJSON(w, r, &req) {
		return
	}
	req.Matricule = strings.TrimSpace(req.Matricule)
	if req.Matricule == "" || strings.TrimSpace(req.Nom) == "" {
		erreur(w, http.StatusBadRequest, "Le matricule et le nom sont obligatoires.", "champs_manquants")
		return
	}
	if req.Role == "" {
		req.Role = "agent"
	}
	if !rolesValides[req.Role] {
		erreur(w, http.StatusBadRequest, "Rôle inconnu. Valeurs acceptées : agent, chef, admin.", "role_invalide")
		return
	}
	// Seul un admin peut créer un autre admin.
	if req.Role == "admin" && !acteur.Peut("admin") {
		erreur(w, http.StatusForbidden, "Seul un administrateur peut créer un compte administrateur.", "droits_insuffisants")
		return
	}

	// Sans mot de passe fourni, on en génère un à remettre à l'agent.
	genere := ""
	if req.MotDePasse == "" {
		var err error
		if genere, err = auth.MotDePasseAleatoire(); err != nil {
			erreurStore(w, err)
			return
		}
		req.MotDePasse = genere
	} else if err := auth.ValiderMotDePasse(req.MotDePasse); err != nil {
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "mot_de_passe_faible")
		return
	}

	hash, err := auth.HashPassword(req.MotDePasse)
	if err != nil {
		erreurStore(w, err)
		return
	}
	u := &store.User{
		Matricule: req.Matricule, Nom: req.Nom, Prenom: req.Prenom, Email: req.Email,
		PasswordHash: hash, Role: req.Role, Actif: true, MustChange: true,
	}
	if err := s.st.CreateUser(u); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(acteur.ID, "creation_compte", "user", u.ID, "matricule="+u.Matricule+" role="+u.Role, s.ipDe(r))

	rep := map[string]any{"user": u}
	if genere != "" {
		// Affiché une seule fois, à transmettre à l'agent.
		rep["mot_de_passe_provisoire"] = genere
	}
	ecrireJSON(w, http.StatusCreated, rep)
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var req requeteUser
	if !decoderJSON(w, r, &req) {
		return
	}
	if req.Matricule != "" {
		u.Matricule = strings.TrimSpace(req.Matricule)
	}
	if req.Nom != "" {
		u.Nom = req.Nom
	}
	if req.Prenom != "" {
		u.Prenom = req.Prenom
	}
	if req.Email != "" {
		u.Email = req.Email
	}
	if req.Role != "" {
		if !rolesValides[req.Role] {
			erreur(w, http.StatusBadRequest, "Rôle inconnu.", "role_invalide")
			return
		}
		if (req.Role == "admin" || u.Role == "admin") && !acteur.Peut("admin") {
			erreur(w, http.StatusForbidden, "Seul un administrateur peut modifier un rôle administrateur.", "droits_insuffisants")
			return
		}
		u.Role = req.Role
	}
	if req.Actif != nil {
		// Garde-fou : ne pas se verrouiller hors de sa propre instance.
		if !*req.Actif && u.ID == acteur.ID {
			erreur(w, http.StatusUnprocessableEntity, "Vous ne pouvez pas désactiver votre propre compte.", "auto_desactivation")
			return
		}
		// Un agent désactivé alors qu'il détient un véhicule laisserait celui-ci
		// bloqué « en service » avec une sortie ouverte au nom de quelqu'un qui
		// ne peut plus se connecter. Le véhicule disparaîtrait du parc
		// disponible sans que rien ne le signale.
		if !*req.Actif {
			if enCours, err := s.st.CheckoutEnCoursPourAgent(u.ID); err == nil {
				erreur(w, http.StatusConflict, fmt.Sprintf(
					"%s détient actuellement le véhicule %s. Clôturez cette sortie avant de désactiver le compte.",
					u.NomComplet(), enCours.VehicleCode), "detient_vehicule")
				return
			}
		}
		if !*req.Actif && u.Role == "admin" {
			nb, err := s.compterAdminsActifs()
			if err != nil {
				erreurStore(w, err)
				return
			}
			if nb <= 1 {
				erreur(w, http.StatusUnprocessableEntity,
					"Impossible de désactiver le dernier administrateur actif.", "dernier_admin")
				return
			}
		}
		u.Actif = *req.Actif
	}
	if err := s.st.UpdateUser(u); err != nil {
		erreurStore(w, err)
		return
	}
	if !u.Actif {
		_ = s.auth.LogoutTous(u.ID) // un compte désactivé perd ses sessions immédiatement
	}
	s.st.Audit(acteur.ID, "modification_compte", "user", u.ID, "", s.ipDe(r))
	ecrireJSON(w, http.StatusOK, u)
}

// postResetMotDePasse : un chef remet à zéro le mot de passe d'un agent.
func (s *Server) postResetMotDePasse(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if u.Role == "admin" && !acteur.Peut("admin") {
		erreur(w, http.StatusForbidden, "Seul un administrateur peut réinitialiser un compte administrateur.", "droits_insuffisants")
		return
	}
	nouveau, err := auth.MotDePasseAleatoire()
	if err != nil {
		erreurStore(w, err)
		return
	}
	hash, err := auth.HashPassword(nouveau)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.st.SetPassword(u.ID, hash, true); err != nil {
		erreurStore(w, err)
		return
	}
	_ = s.auth.LogoutTous(u.ID)
	s.st.Audit(acteur.ID, "reinitialisation_mot_de_passe", "user", u.ID, "", s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]string{
		"mot_de_passe_provisoire": nouveau,
		"message":                 "Transmettez ce mot de passe à l'agent : il ne sera plus affiché.",
	})
}

func (s *Server) compterAdminsActifs() (int, error) {
	var n int
	err := s.st.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin' AND actif=1`).Scan(&n)
	return n, err
}

// delaiLisible met un délai en français, arrondi à la minute supérieure : une
// consigne « réessayez dans 14 minutes » est plus utile que « dans 13m47s ».
func delaiLisible(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	if d > 0 && minutes < 1 {
		minutes = 1
	}
	if minutes <= 1 {
		return "une minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// --- Sessions actives ---

// getSessions liste les appareils connectés au compte courant.
func (s *Server) getSessions(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	sessions, err := s.st.SessionsDeLUtilisateur(u.ID, auth.HashJeton(jetonDe(r)))
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, sessions)
}

// deleteSession coupe un appareil précis : c'est le geste à faire quand un
// téléphone est perdu, sans attendre qu'un chef désactive tout le compte.
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	if err := s.st.RevoquerSession(u.ID, id); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "session_revoquee", "session", id, "", s.ipDe(r))
	w.WriteHeader(http.StatusNoContent)
}

// postRevoquerAutresSessions coupe tout sauf l'appareil courant.
func (s *Server) postRevoquerAutresSessions(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	n, err := s.st.RevoquerAutresSessions(u.ID, auth.HashJeton(jetonDe(r)))
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "sessions_revoquees", "user", u.ID,
		fmt.Sprintf("%d session(s)", n), s.ipDe(r))
	ecrireJSON(w, http.StatusOK, map[string]any{"revoquees": n})
}
