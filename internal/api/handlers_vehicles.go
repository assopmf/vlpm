package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/assopmf/vlpm/internal/store"
)

var statutsValides = map[string]bool{
	"disponible": true, "en_service": true, "maintenance": true, "hors_service": true,
}

func (s *Server) getVehicles(w http.ResponseWriter, r *http.Request) {
	statut := r.URL.Query().Get("statut")
	if statut != "" && statut != "tous" && !statutsValides[statut] {
		erreur(w, http.StatusBadRequest, "Statut inconnu.", "statut_invalide")
		return
	}
	vs, err := s.st.ListVehicles(statut, r.URL.Query().Get("archives") == "1")
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, vs)
}

// getVehicle renvoie la fiche complète : véhicule, prise en compte en cours,
// derniers mouvements, incidents et entretiens.
func (s *Server) getVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	v, err := s.st.VehicleByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	u := utilisateurDe(r)
	if !u.Peut("chef") {
		v.QRToken = "" // le jeton du QR n'est utile qu'aux gestionnaires
	}

	rep := map[string]any{"vehicule": v}
	if c, err := s.st.CheckoutEnCours(v.ID); err == nil {
		rep["checkout_en_cours"] = c
	}
	if hist, err := s.st.ListCheckouts(store.CheckoutFiltre{VehicleID: v.ID, Limit: 20}); err == nil {
		rep["historique"] = hist
	}
	if inc, err := s.st.ListIncidents(v.ID, "tous"); err == nil {
		rep["incidents"] = inc
	}
	if mt, err := s.st.ListMaintenances(v.ID); err == nil {
		rep["maintenances"] = mt
	}
	ecrireJSON(w, http.StatusOK, rep)
}

type requeteVehicule struct {
	Code                string `json:"code"`
	Marque              string `json:"marque"`
	Modele              string `json:"modele"`
	Immatriculation     string `json:"immatriculation"`
	Categorie           string `json:"categorie"`
	Statut              string `json:"statut"`
	KM                  *int64 `json:"km"`
	DateMiseCirculation string `json:"date_mise_circulation"`
	ProchainCT          string `json:"prochain_ct"`
	ProchaineRevisionKM *int64 `json:"prochaine_revision_km"`
	Notes               string `json:"notes"`
	Archive             *bool  `json:"archive"`
}

func (s *Server) postVehicles(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	var req requeteVehicule
	if !decoderJSON(w, r, &req) {
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		erreur(w, http.StatusBadRequest, "Le code du véhicule (ex. TV1) est obligatoire.", "champs_manquants")
		return
	}
	if req.Statut == "" {
		req.Statut = "disponible"
	}
	if !statutsValides[req.Statut] {
		erreur(w, http.StatusBadRequest, "Statut inconnu.", "statut_invalide")
		return
	}
	if req.Categorie == "" {
		req.Categorie = "patrouille"
	}
	jeton, err := nouveauQRToken()
	if err != nil {
		erreurStore(w, err)
		return
	}
	v := &store.Vehicle{
		Code: req.Code, Marque: req.Marque, Modele: req.Modele,
		Immatriculation: strings.ToUpper(strings.TrimSpace(req.Immatriculation)),
		Categorie:       req.Categorie, Statut: req.Statut,
		DateMiseCirculation: req.DateMiseCirculation, ProchainCT: req.ProchainCT,
		ProchaineRevisionKM: req.ProchaineRevisionKM, QRToken: jeton, Notes: req.Notes,
	}
	if req.KM != nil {
		v.KM = *req.KM
	}
	if err := s.st.CreateVehicle(v); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(acteur.ID, "creation_vehicule", "vehicle", v.ID, v.Code, s.ipDe(r))
	ecrireJSON(w, http.StatusCreated, v)
}

func (s *Server) patchVehicle(w http.ResponseWriter, r *http.Request) {
	acteur := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	v, err := s.st.VehicleByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var req requeteVehicule
	if !decoderJSON(w, r, &req) {
		return
	}
	if req.Code != "" {
		v.Code = strings.TrimSpace(req.Code)
	}
	if req.Marque != "" {
		v.Marque = req.Marque
	}
	if req.Modele != "" {
		v.Modele = req.Modele
	}
	if req.Immatriculation != "" {
		v.Immatriculation = strings.ToUpper(strings.TrimSpace(req.Immatriculation))
	}
	if req.Categorie != "" {
		v.Categorie = req.Categorie
	}
	if req.Statut != "" {
		if !statutsValides[req.Statut] {
			erreur(w, http.StatusBadRequest, "Statut inconnu.", "statut_invalide")
			return
		}
		// "en_service" est piloté par les prises en compte, jamais à la main :
		// sinon le statut et l'historique divergent silencieusement.
		if req.Statut == "en_service" || v.Statut == "en_service" {
			erreur(w, http.StatusConflict,
				"Le statut « en service » découle des prises en compte. Clôturez la sortie en cours pour changer le statut.",
				"statut_pilote")
			return
		}
		v.Statut = req.Statut
	}
	if req.KM != nil {
		if *req.KM < v.KM {
			erreur(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("Le kilométrage ne peut pas diminuer (compteur actuel : %s km).",
					store.FmtKM(v.KM)), "km_incoherent")
			return
		}
		v.KM = *req.KM
	}
	if req.DateMiseCirculation != "" {
		v.DateMiseCirculation = req.DateMiseCirculation
	}
	if req.ProchainCT != "" {
		v.ProchainCT = req.ProchainCT
	}
	if req.ProchaineRevisionKM != nil {
		v.ProchaineRevisionKM = req.ProchaineRevisionKM
	}
	if req.Notes != "" {
		v.Notes = req.Notes
	}
	if req.Archive != nil {
		if *req.Archive && v.Statut == "en_service" {
			erreur(w, http.StatusConflict, "Impossible d'archiver un véhicule en service.", "en_service")
			return
		}
		v.Archive = *req.Archive
	}
	if err := s.st.UpdateVehicle(v); err != nil {
		erreurStore(w, err)
		return
	}
	s.st.Audit(acteur.ID, "modification_vehicule", "vehicle", v.ID, v.Code, s.ipDe(r))
	ecrireJSON(w, http.StatusOK, v)
}

// getQRCode produit l'étiquette PNG à coller dans le véhicule.
func (s *Server) getQRCode(w http.ResponseWriter, r *http.Request) {
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	v, err := s.st.VehicleByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	taille := queryInt(r, "taille", 512)
	if taille < 128 {
		taille = 128
	}
	if taille > 1024 {
		taille = 1024
	}

	png, err := qrcode.Encode(s.urlScan(v.QRToken), qrcode.Medium, taille)
	if err != nil {
		erreurStore(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "qr-"+v.Code+".png"))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(png)
}

// urlScan construit le lien encodé dans le QR. Sans base-url configurée, on
// encode un lien relatif : l'agent devra ouvrir l'app d'abord, mais l'étiquette
// reste valable si l'adresse du serveur change.
func (s *Server) urlScan(jeton string) string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL + "/scan/" + jeton
	}
	return "vlpm:" + jeton
}

// getScan résout un jeton de QR vers le véhicule correspondant.
func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	jeton := strings.TrimSpace(r.PathValue("token"))
	// Un QR peut avoir été scanné sous forme d'URL complète : on ne garde que
	// le dernier segment, et le préfixe "vlpm:" du mode hors-ligne.
	jeton = strings.TrimPrefix(jeton, "vlpm:")
	if i := strings.LastIndex(jeton, "/"); i >= 0 {
		jeton = jeton[i+1:]
	}
	if jeton == "" {
		erreur(w, http.StatusBadRequest, "QR code illisible.", "qr_invalide")
		return
	}
	v, err := s.st.VehicleByQR(jeton)
	if err != nil {
		erreur(w, http.StatusNotFound, "Ce QR code ne correspond à aucun véhicule de votre parc.", "qr_inconnu")
		return
	}
	v.QRToken = ""
	rep := map[string]any{"vehicule": v}
	if c, err := s.st.CheckoutEnCours(v.ID); err == nil {
		rep["checkout_en_cours"] = c
	}
	ecrireJSON(w, http.StatusOK, rep)
}

func nouveauQRToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// --- Exports CSV ---
//
// Tous les exports partagent le même format : UTF-8 avec BOM et séparateur
// point-virgule, ce qu'attend Excel en configuration française. Sans le BOM,
// les accents sont illisibles ; sans le point-virgule, tout atterrit dans une
// seule colonne.

// ecrireCSV envoie un tableau en pièce jointe. Le nom du fichier porte la date
// du jour, pour que plusieurs exports successifs ne s'écrasent pas.
func ecrireCSV(w http.ResponseWriter, sujet string, entetes []string, lignes [][]string) {
	nom := fmt.Sprintf("vlpm-%s-%s.csv", sujet, time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", nom))
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	c := csv.NewWriter(w)
	c.Comma = ';'
	defer c.Flush()
	c.Write(entetes)
	for _, l := range lignes {
		c.Write(l)
	}
}

// getExportCSV exporte l'historique des prises en compte, pour l'archivage
// communal ou un tableur.
func (s *Server) getExportCSV(w http.ResponseWriter, r *http.Request) {
	f := store.CheckoutFiltre{
		VehicleID: int64(queryInt(r, "vehicule", 0)),
		Statut:    r.URL.Query().Get("statut"),
		Depuis:    r.URL.Query().Get("depuis"),
		Jusqua:    r.URL.Query().Get("jusqua"),
		Limit:     5000,
	}
	prises, err := s.st.ListCheckouts(f)
	if err != nil {
		erreurStore(w, err)
		return
	}

	lignes := make([][]string, 0, len(prises))
	for _, l := range prises {
		lignes = append(lignes, []string{
			l.VehicleCode, l.UserNom, dateFR(l.StartedAt), entier(l.KMStart),
			dateFR(l.EndedAt), entierPtr(l.KMEnd), entierPtr(l.Distance),
			l.Motif, l.NotesDepart, l.NotesRetour,
			ouiNon(l.DepartHorsLigne || l.RetourHorsLigne),
		})
	}
	ecrireCSV(w, "historique", []string{
		"Véhicule", "Agent", "Départ", "KM départ", "Retour", "KM retour",
		"Distance (km)", "Motif", "Observations départ", "Observations retour",
		"Saisi hors ligne",
	}, lignes)
}

// getExportVehicules exporte l'état du parc : inventaire communal, échéances.
func (s *Server) getExportVehicules(w http.ResponseWriter, r *http.Request) {
	vs, err := s.st.ListVehicles("", r.URL.Query().Get("archives") == "1")
	if err != nil {
		erreurStore(w, err)
		return
	}

	lignes := make([][]string, 0, len(vs))
	for _, v := range vs {
		detenteur := ""
		if v.CheckoutEnCours != nil {
			detenteur = v.CheckoutEnCours.UserNom
		}
		lignes = append(lignes, []string{
			v.Code, v.Marque, v.Modele, v.Immatriculation, v.Categorie,
			libelleStatut(v.Statut), entier(v.KM),
			dateFRCourte(v.DateMiseCirculation), dateFRCourte(v.ProchainCT),
			entierPtr(v.ProchaineRevisionKM), detenteur,
			entier(int64(v.IncidentsOuverts)), v.Notes,
		})
	}
	ecrireCSV(w, "parc", []string{
		"Code", "Marque", "Modèle", "Immatriculation", "Catégorie", "Statut",
		"Kilométrage", "Mise en circulation", "Prochain CT", "Prochaine révision (km)",
		"Détenteur actuel", "Incidents ouverts", "Notes",
	}, lignes)
}

// getExportIncidents exporte les signalements, pour un suivi de sinistralité.
func (s *Server) getExportIncidents(w http.ResponseWriter, r *http.Request) {
	statut := r.URL.Query().Get("statut")
	if statut == "" {
		statut = "tous"
	}
	incidents, err := s.st.ListIncidents(int64(queryInt(r, "vehicule", 0)), statut)
	if err != nil {
		erreurStore(w, err)
		return
	}

	lignes := make([][]string, 0, len(incidents))
	for _, i := range incidents {
		lignes = append(lignes, []string{
			i.VehicleCode, dateFR(i.CreatedAt), i.UserNom,
			libelleType(i.Type), libelleGravite(i.Gravite), i.Description,
			libelleStatutIncident(i.Statut), dateFR(i.ResoluAt),
		})
	}
	ecrireCSV(w, "incidents", []string{
		"Véhicule", "Signalé le", "Signalé par", "Nature", "Gravité",
		"Description", "Statut", "Résolu le",
	}, lignes)
}

// getExportEntretiens exporte les entretiens et leurs coûts, pour le budget.
func (s *Server) getExportEntretiens(w http.ResponseWriter, r *http.Request) {
	entretiens, err := s.st.ListMaintenances(int64(queryInt(r, "vehicule", 0)))
	if err != nil {
		erreurStore(w, err)
		return
	}

	var total int64
	lignes := make([][]string, 0, len(entretiens)+1)
	for _, m := range entretiens {
		total += m.CoutCents
		lignes = append(lignes, []string{
			m.VehicleCode, dateFRCourte(m.Date), libelleEntretien(m.Type),
			entierPtr(m.KM), montant(m.CoutCents), m.Prestataire, m.Description,
		})
	}
	// Ligne de total : un export destiné au budget doit se suffire à lui-même.
	if len(lignes) > 0 {
		lignes = append(lignes, []string{"", "", "TOTAL", "", montant(total), "", ""})
	}
	ecrireCSV(w, "entretiens", []string{
		"Véhicule", "Date", "Nature", "Kilométrage", "Coût (€)", "Prestataire", "Description",
	}, lignes)
}

// --- Mise en forme des colonnes ---

func entier(n int64) string { return strconv.FormatInt(n, 10) }

func entierPtr(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}

// montant utilise la virgule décimale, seule forme reconnue comme un nombre
// par Excel en configuration française.
func montant(centimes int64) string {
	return fmt.Sprintf("%d,%02d", centimes/100, centimes%100)
}

func ouiNon(b bool) string {
	if b {
		return "oui"
	}
	return "non"
}

func dateFRCourte(iso string) string {
	if iso == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02/01/2006")
	}
	return dateFR(iso)
}

var (
	libellesStatut = map[string]string{
		"disponible": "Disponible", "en_service": "En service",
		"maintenance": "Maintenance", "hors_service": "Hors service",
	}
	libellesType = map[string]string{
		"dommage": "Dommage", "panne": "Panne", "proprete": "Propreté",
		"carburant": "Carburant", "equipement": "Équipement", "autre": "Autre",
	}
	libellesGravite = map[string]string{
		"mineur": "Mineur", "majeur": "Majeur", "immobilisant": "Immobilisant",
	}
	libellesStatutIncident = map[string]string{
		"ouvert": "Ouvert", "en_cours": "En cours de traitement", "resolu": "Résolu",
	}
	libellesEntretien = map[string]string{
		"revision": "Révision", "reparation": "Réparation",
		"controle_technique": "Contrôle technique", "pneus": "Pneumatiques",
		"carburant": "Carburant", "autre": "Autre",
	}
)

func libelle(m map[string]string, cle string) string {
	if v, ok := m[cle]; ok {
		return v
	}
	return cle
}

func libelleStatut(v string) string         { return libelle(libellesStatut, v) }
func libelleType(v string) string           { return libelle(libellesType, v) }
func libelleGravite(v string) string        { return libelle(libellesGravite, v) }
func libelleStatutIncident(v string) string { return libelle(libellesStatutIncident, v) }
func libelleEntretien(v string) string      { return libelle(libellesEntretien, v) }

func dateFR(iso string) string {
	if iso == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.Local().Format("02/01/2006 15:04")
}
