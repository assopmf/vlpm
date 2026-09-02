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
				fmt.Sprintf("Le kilométrage ne peut pas diminuer (compteur actuel : %d km).", v.KM), "km_incoherent")
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

// getExportCSV exporte l'historique des prises en compte, pour l'archivage
// communal ou un tableur.
func (s *Server) getExportCSV(w http.ResponseWriter, r *http.Request) {
	f := store.CheckoutFiltre{
		VehicleID: int64(queryInt(r, "vehicule", 0)),
		Statut:    r.URL.Query().Get("statut"),
		Depuis:    r.URL.Query().Get("depuis"),
		Jusqua:    r.URL.Query().Get("jusqua"),
		Limit:     500,
	}
	lignes, err := s.st.ListCheckouts(f)
	if err != nil {
		erreurStore(w, err)
		return
	}
	nom := fmt.Sprintf("vlpm-historique-%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", nom))
	// BOM UTF-8 : sans lui, Excel en français massacre les accents.
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	c := csv.NewWriter(w)
	c.Comma = ';' // séparateur attendu par Excel en configuration française
	defer c.Flush()
	c.Write([]string{"Véhicule", "Agent", "Départ", "KM départ", "Retour", "KM retour",
		"Distance (km)", "Motif", "Observations départ", "Observations retour"})
	for _, l := range lignes {
		kmEnd, dist := "", ""
		if l.KMEnd != nil {
			kmEnd = strconv.FormatInt(*l.KMEnd, 10)
		}
		if l.Distance != nil {
			dist = strconv.FormatInt(*l.Distance, 10)
		}
		c.Write([]string{l.VehicleCode, l.UserNom, dateFR(l.StartedAt),
			strconv.FormatInt(l.KMStart, 10), dateFR(l.EndedAt), kmEnd, dist,
			l.Motif, l.NotesDepart, l.NotesRetour})
	}
}

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
