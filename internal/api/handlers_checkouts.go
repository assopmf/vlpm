package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/assopmf/vlpm/internal/store"
)

func (s *Server) getCheckouts(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	f := store.CheckoutFiltre{
		VehicleID: int64(queryInt(r, "vehicule", 0)),
		UserID:    int64(queryInt(r, "agent", 0)),
		Statut:    r.URL.Query().Get("statut"),
		Depuis:    r.URL.Query().Get("depuis"),
		Jusqua:    r.URL.Query().Get("jusqua"),
		Limit:     queryInt(r, "limite", 100),
		Offset:    queryInt(r, "offset", 0),
	}
	// Un agent ne consulte que son propre historique ; chefs et admins voient tout.
	if !u.Peut("chef") {
		f.UserID = u.ID
	}
	liste, err := s.st.ListCheckouts(f)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, liste)
}

type requetePrise struct {
	VehiculeID int64           `json:"vehicule_id"`
	QRToken    string          `json:"qr_token"`
	KM         *int64          `json:"km"`
	Motif      string          `json:"motif"`
	Notes      string          `json:"notes"`
	Check      json.RawMessage `json:"check"`
	Force      bool            `json:"force"`

	// AgentID permet à un chef d'enregistrer la sortie au nom d'un agent qui
	// ne peut pas le faire lui-même : téléphone oublié, saisie au poste.
	AgentID int64 `json:"agent_id"`

	// Renseignés par un client qui rejoue une saisie faite sans réseau.
	CleClient string `json:"cle_client"`
	DateDebut string `json:"date_debut"`
	HorsLigne bool   `json:"hors_ligne"`
}

// postPriseEnCompte : un agent prend un véhicule, par scan du QR ou depuis la liste.
func (s *Server) postPriseEnCompte(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requetePrise
	if !decoderJSON(w, r, &req) {
		return
	}

	// Le rejeu se traite avant toute validation métier : l'opération a déjà
	// abouti, les gardes qui suivent n'ont pas à s'y appliquer.
	if cle := strings.TrimSpace(req.CleClient); cle != "" {
		if dejaVu, err := s.st.CheckoutParCleClient(cle); err == nil {
			ecrireJSON(w, http.StatusOK, dejaVu)
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			erreurStore(w, err)
			return
		}
	}

	vehiculeID := req.VehiculeID
	if vehiculeID == 0 && req.QRToken != "" {
		v, err := s.st.VehicleByQR(strings.TrimPrefix(strings.TrimSpace(req.QRToken), "vlpm:"))
		if err != nil {
			erreur(w, http.StatusNotFound, "Ce QR code ne correspond à aucun véhicule.", "qr_inconnu")
			return
		}
		vehiculeID = v.ID
	}
	if vehiculeID == 0 {
		erreur(w, http.StatusBadRequest, "Indiquez le véhicule (identifiant ou QR code).", "champs_manquants")
		return
	}
	if req.KM == nil {
		erreur(w, http.StatusBadRequest, "Le kilométrage au départ est obligatoire.", "km_manquant")
		return
	}
	if *req.KM < 0 {
		erreur(w, http.StatusBadRequest, "Le kilométrage ne peut pas être négatif.", "km_incoherent")
		return
	}

	// Détenteur de la sortie : l'utilisateur connecté, sauf si un chef la
	// saisit au nom d'un agent. Le contrôle du détenteur unique est fait dans
	// la transaction, pour résister aux saisies concurrentes.
	detenteur := u
	if req.AgentID > 0 && req.AgentID != u.ID {
		if !u.Peut("chef") {
			erreur(w, http.StatusForbidden,
				"Seul un chef de service peut enregistrer une sortie au nom d'un autre agent.",
				"droits_insuffisants")
			return
		}
		agent, err := s.st.UserByID(req.AgentID)
		if err != nil {
			erreur(w, http.StatusNotFound, "Cet agent est introuvable.", "agent_introuvable")
			return
		}
		if !agent.Actif {
			erreur(w, http.StatusUnprocessableEntity,
				"Le compte de cet agent est désactivé : il ne peut pas se voir confier un véhicule.",
				"compte_inactif")
			return
		}
		detenteur = agent
	}

	debut, err := horodatage(req.DateDebut)
	if err != nil {
		erreur(w, http.StatusBadRequest, err.Error(), "date_invalide")
		return
	}

	c, err := s.st.PrendreEnCompte(store.PriseEnCompte{
		VehicleID: vehiculeID, UserID: detenteur.ID, KMStart: *req.KM,
		Motif: req.Motif, Notes: req.Notes, Check: jsonOuVide(req.Check), Force: req.Force,
		CleClient: strings.TrimSpace(req.CleClient), DebutDeclare: debut, HorsLigne: req.HorsLigne,
		SaisiPar: u.ID,
	})
	if errors.Is(err, store.ErrDejaEnregistre) {
		// Rejeu : l'opération avait déjà abouti. On répond un succès pour que
		// le client la retire de sa file au lieu de réessayer indéfiniment.
		ecrireJSON(w, http.StatusOK, c)
		return
	}
	if err != nil {
		erreurStore(w, err)
		return
	}
	details := fmt.Sprintf("%s à %s km", c.VehicleCode, store.FmtKM(c.KMStart))
	if detenteur.ID != u.ID {
		details += fmt.Sprintf(" — saisi au nom de %s", detenteur.NomComplet())
	}
	s.st.Audit(u.ID, "prise_en_compte", "checkout", c.ID, details, s.ipDe(r))
	ecrireJSON(w, http.StatusCreated, c)
}

type requeteRestitution struct {
	KM         *int64          `json:"km"`
	Notes      string          `json:"notes"`
	Check      json.RawMessage `json:"check"`
	Immobilise bool            `json:"immobilise"`
	Force      bool            `json:"force"`
	CleClient  string          `json:"cle_client"`
	DateRetour string          `json:"date_retour"`
	HorsLigne  bool            `json:"hors_ligne"`
	Incidents  []struct {
		Type        string `json:"type"`
		Gravite     string `json:"gravite"`
		Description string `json:"description"`
	} `json:"incidents"`
}

// postRestitution clôt une sortie. Un agent ne peut clôturer que la sienne ;
// un chef peut clôturer celle d'un autre (fin de service, agent absent).
func (s *Server) postRestitution(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	existant, err := s.st.CheckoutByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if existant.UserID != u.ID && !u.Peut("chef") {
		erreur(w, http.StatusForbidden,
			"Cette prise en compte est au nom d'un autre agent. Seul un chef de service peut la clôturer.",
			"droits_insuffisants")
		return
	}

	var req requeteRestitution
	if !decoderJSON(w, r, &req) {
		return
	}
	if req.KM == nil {
		erreur(w, http.StatusBadRequest, "Le kilométrage au retour est obligatoire.", "km_manquant")
		return
	}

	// Un incident immobilisant envoie automatiquement le véhicule en maintenance.
	immobilise := req.Immobilise
	for _, inc := range req.Incidents {
		if inc.Gravite == "immobilisant" {
			immobilise = true
		}
	}

	var clotureePar int64
	if existant.UserID != u.ID {
		clotureePar = u.ID
	}
	retour, err := horodatage(req.DateRetour)
	if err != nil {
		erreur(w, http.StatusBadRequest, err.Error(), "date_invalide")
		return
	}

	c, err := s.st.Restituer(store.Restitution{
		CheckoutID: id, KMEnd: *req.KM, Notes: req.Notes, Check: jsonOuVide(req.Check),
		Immobilise: immobilise, ClotureePar: clotureePar, Force: req.Force,
		CleClient: strings.TrimSpace(req.CleClient), RetourDeclare: retour, HorsLigne: req.HorsLigne,
	})
	if errors.Is(err, store.ErrDejaEnregistre) {
		ecrireJSON(w, http.StatusOK, c)
		return
	}
	if err != nil {
		erreurStore(w, err)
		return
	}

	for _, inc := range req.Incidents {
		if strings.TrimSpace(inc.Description) == "" {
			continue
		}
		i := &store.Incident{
			VehicleID: c.VehicleID, CheckoutID: &c.ID, UserID: u.ID,
			Type: defaut(inc.Type, "autre"), Gravite: defaut(inc.Gravite, "mineur"),
			Description: inc.Description,
		}
		if err := s.st.CreateIncident(i); err != nil {
			s.log.Error("enregistrement d'un incident au retour", "erreur", err, "checkout", c.ID)
		}
	}

	details := fmt.Sprintf("%s à %d km", c.VehicleCode, *req.KM)
	if c.Distance != nil {
		details += fmt.Sprintf(" (%d km parcourus)", *c.Distance)
	}
	s.st.Audit(u.ID, "restitution", "checkout", c.ID, details, s.ipDe(r))
	ecrireJSON(w, http.StatusOK, c)
}

// --- Incidents ---

func (s *Server) getIncidents(w http.ResponseWriter, r *http.Request) {
	statut := r.URL.Query().Get("statut")
	if statut == "" {
		statut = "ouverts"
	}
	liste, err := s.st.ListIncidents(int64(queryInt(r, "vehicule", 0)), statut)
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, liste)
}

type requeteIncident struct {
	VehiculeID  int64  `json:"vehicule_id"`
	Type        string `json:"type"`
	Gravite     string `json:"gravite"`
	Description string `json:"description"`
	CleClient   string `json:"cle_client"`
}

func (s *Server) postIncidents(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteIncident
	if !decoderJSON(w, r, &req) {
		return
	}
	if req.VehiculeID == 0 || strings.TrimSpace(req.Description) == "" {
		erreur(w, http.StatusBadRequest, "Le véhicule et la description sont obligatoires.", "champs_manquants")
		return
	}
	i := &store.Incident{
		VehicleID: req.VehiculeID, UserID: u.ID,
		Type: defaut(req.Type, "autre"), Gravite: defaut(req.Gravite, "mineur"),
		Description: req.Description, CleClient: strings.TrimSpace(req.CleClient),
	}
	if err := s.st.CreateIncident(i); errors.Is(err, store.ErrDejaEnregistre) {
		ecrireJSON(w, http.StatusOK, i)
		return
	} else if err != nil {
		erreurStore(w, err)
		return
	}
	if i.Gravite == "immobilisant" {
		// Le véhicule ne doit plus être repris tant que l'incident est ouvert.
		if v, err := s.st.VehicleByID(i.VehicleID); err == nil && v.Statut == "disponible" {
			_ = s.st.SetStatut(v.ID, "maintenance")
		}
	}
	s.st.Audit(u.ID, "signalement_incident", "incident", i.ID, i.Gravite+" : "+i.Description, s.ipDe(r))
	ecrireJSON(w, http.StatusCreated, i)
}

func (s *Server) postResoudreIncident(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	if err := s.st.ResoudreIncident(id, u.ID); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "resolution_incident", "incident", id, "", s.ipDe(r))
	w.WriteHeader(http.StatusNoContent)
}

// --- Entretiens ---

func (s *Server) getMaintenances(w http.ResponseWriter, r *http.Request) {
	liste, err := s.st.ListMaintenances(int64(queryInt(r, "vehicule", 0)))
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, liste)
}

type requeteMaintenance struct {
	VehiculeID        int64  `json:"vehicule_id"`
	Type              string `json:"type"`
	Date              string `json:"date"`
	KM                *int64 `json:"km"`
	CoutEuros         string `json:"cout_euros"`
	Prestataire       string `json:"prestataire"`
	Description       string `json:"description"`
	RemettreEnService bool   `json:"remettre_en_service"`
}

func (s *Server) postMaintenances(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	var req requeteMaintenance
	if !decoderJSON(w, r, &req) {
		return
	}
	if req.VehiculeID == 0 {
		erreur(w, http.StatusBadRequest, "Le véhicule est obligatoire.", "champs_manquants")
		return
	}
	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}
	cout, err := eurosEnCentimes(req.CoutEuros)
	if err != nil {
		erreur(w, http.StatusBadRequest, "Montant invalide : "+err.Error(), "montant_invalide")
		return
	}
	m := &store.Maintenance{
		VehicleID: req.VehiculeID, Type: defaut(req.Type, "autre"), Date: req.Date,
		KM: req.KM, CoutCents: cout, Prestataire: req.Prestataire, Description: req.Description,
	}
	if err := s.st.CreateMaintenance(m, u.ID); err != nil {
		erreurStore(w, err)
		return
	}
	if req.RemettreEnService {
		v, err := s.st.VehicleByID(req.VehiculeID)
		if err == nil && v.Statut == "maintenance" {
			_ = s.st.SetStatut(v.ID, "disponible")
		}
	}
	s.st.Audit(u.ID, "entretien", "maintenance", m.ID, m.Type+" "+m.Description, s.ipDe(r))
	ecrireJSON(w, http.StatusCreated, m)
}

// eurosEnCentimes accepte "150", "150,50" et "150.50".
func eurosEnCentimes(v string) (int64, error) {
	v = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(v, ",", "."), " ", ""))
	if v == "" {
		return 0, nil
	}
	var euros, centimes int64
	partie := strings.SplitN(v, ".", 2)
	if _, err := fmt.Sscanf(partie[0], "%d", &euros); err != nil {
		return 0, fmt.Errorf("« %s » n'est pas un montant", v)
	}
	if len(partie) == 2 {
		dec := (partie[1] + "00")[:2]
		if _, err := fmt.Sscanf(dec, "%d", &centimes); err != nil {
			return 0, fmt.Errorf("« %s » n'est pas un montant", v)
		}
	}
	if euros < 0 {
		return 0, fmt.Errorf("le montant ne peut pas être négatif")
	}
	return euros*100 + centimes, nil
}

// --- Tableau de bord ---

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stats()
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, st)
}

func (s *Server) getAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.st.ListAudit(queryInt(r, "limite", 200))
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, entries)
}

func jsonOuVide(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func defaut(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// horodatage lit une date RFC 3339 fournie par un client hors ligne. Une valeur
// absente laisse le serveur employer sa propre horloge.
func horodatage(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("Date « %s » illisible : le format attendu est 2026-09-02T14:07:00Z.", v)
	}
	return t, nil
}
