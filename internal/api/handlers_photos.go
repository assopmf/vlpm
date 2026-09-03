package api

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	// Importés pour leur effet de bord : chaque paquet enregistre son format
	// auprès de image.DecodeConfig, ce qui permet de reconnaître un fichier à
	// son en-tête plutôt qu'au type déclaré par le client.
	_ "golang.org/x/image/webp"

	"github.com/assopmf/vlpm/internal/store"
)

// TaillePhotoMax borne un envoi. L'interface réduit déjà les images avant
// transmission ; cette limite protège des clients qui ne le feraient pas.
const TaillePhotoMax = 8 << 20 // 8 Mio

// typesImageAcceptes associe le type détecté à son extension. Le type est
// déterminé en lisant l'en-tête du fichier, jamais d'après ce que le client
// déclare : un client peut annoncer image/jpeg et envoyer autre chose.
var typesImageAcceptes = map[string]string{
	"jpeg": ".jpg",
	"png":  ".png",
	"webp": ".webp",
}

func (s *Server) dossierPhotos() string {
	return filepath.Join(s.cfg.DataDir, "photos")
}

// postPhoto reçoit une photo jointe à un constat d'incident.
func (s *Server) postPhoto(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	incidentID, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	incidents, err := s.st.ListIncidents(0, "tous")
	if err != nil {
		erreurStore(w, err)
		return
	}
	trouve := false
	for _, i := range incidents {
		if i.ID == incidentID {
			trouve = true
			break
		}
	}
	if !trouve {
		erreur(w, http.StatusNotFound, "Cet incident est introuvable.", "introuvable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, TaillePhotoMax+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		erreur(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Photo trop volumineuse (maximum %d Mo).", TaillePhotoMax>>20), "photo_trop_grande")
		return
	}
	fichier, entete, err := r.FormFile("photo")
	if err != nil {
		erreur(w, http.StatusBadRequest, "Aucune photo reçue.", "photo_manquante")
		return
	}
	defer fichier.Close()

	if entete.Size > TaillePhotoMax {
		erreur(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Photo trop volumineuse (maximum %d Mo).", TaillePhotoMax>>20), "photo_trop_grande")
		return
	}

	// Le format est déterminé en décodant l'en-tête : c'est la seule preuve
	// que le fichier est bien une image, et de quel type.
	config, format, err := image.DecodeConfig(fichier)
	if err != nil {
		erreur(w, http.StatusUnprocessableEntity,
			"Ce fichier n'est pas une image exploitable (formats acceptés : JPEG, PNG, WebP).",
			"format_invalide")
		return
	}
	extension, accepte := typesImageAcceptes[format]
	if !accepte {
		erreur(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("Format %s non accepté. Utilisez JPEG, PNG ou WebP.", format),
			"format_invalide")
		return
	}
	if _, err := fichier.Seek(0, io.SeekStart); err != nil {
		erreurStore(w, err)
		return
	}

	nom, err := store.NomFichierPhoto(extension)
	if err != nil {
		erreurStore(w, err)
		return
	}
	dossier := s.dossierPhotos()
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		erreurStore(w, err)
		return
	}
	chemin := filepath.Join(dossier, nom)

	// 0600 : une photo de constat peut montrer une plaque, un lieu, des tiers.
	dst, err := os.OpenFile(chemin, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		erreurStore(w, err)
		return
	}
	octets, err := io.Copy(dst, io.LimitReader(fichier, TaillePhotoMax))
	fermeErr := dst.Close()
	if err == nil {
		err = fermeErr
	}
	if err != nil {
		os.Remove(chemin) // ne pas laisser de fichier partiel
		erreurStore(w, err)
		return
	}

	p := &store.Photo{
		IncidentID: incidentID, Fichier: nom, TypeMIME: "image/" + format,
		Octets: octets, Largeur: config.Width, Hauteur: config.Height,
	}
	if err := s.st.CreatePhoto(p, u.ID); err != nil {
		os.Remove(chemin) // la ligne n'existe pas : le fichier n'a pas à rester
		erreurStore(w, err)
		return
	}
	s.st.Audit(u.ID, "photo_ajoutee", "incident", incidentID,
		fmt.Sprintf("%d octets", octets), s.ipDe(r))
	ecrireJSON(w, http.StatusCreated, p)
}

// getPhoto sert le fichier. L'accès exige une session : une photo de constat
// n'est pas publique.
func (s *Server) getPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	p, _, err := s.st.PhotoByID(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	chemin, err := store.CheminPhoto(s.dossierPhotos(), p.Fichier)
	if err != nil {
		erreurStore(w, err)
		return
	}
	f, err := os.Open(chemin)
	if err != nil {
		// La ligne existe mais le fichier a disparu : disque restauré sans les
		// photos, suppression manuelle. On le dit plutôt que de servir un 500.
		erreur(w, http.StatusNotFound,
			"Le fichier de cette photo est introuvable sur le serveur.", "fichier_absent")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", p.TypeMIME)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", "inline")
	// Une photo est immuable : son identifiant suffit comme ETag.
	w.Header().Set("ETag", fmt.Sprintf("%q", fmt.Sprint(p.ID)))
	if r.Header.Get("If-None-Match") == fmt.Sprintf("%q", fmt.Sprint(p.ID)) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	io.Copy(w, f)
}

// deletePhoto retire une photo. Réservé aux chefs : une pièce de constat ne
// doit pas pouvoir disparaître sur décision d'un seul agent.
func (s *Server) deletePhoto(w http.ResponseWriter, r *http.Request) {
	u := utilisateurDe(r)
	id, ok := idPath(r, "id")
	if !ok {
		erreur(w, http.StatusBadRequest, "Identifiant invalide.", "id_invalide")
		return
	}
	fichier, err := s.st.SupprimerPhoto(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if chemin, err := store.CheminPhoto(s.dossierPhotos(), fichier); err == nil {
		if err := os.Remove(chemin); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("suppression du fichier photo", "erreur", err, "fichier", fichier)
		}
	}
	s.st.Audit(u.ID, "photo_supprimee", "photo", id, "", s.ipDe(r))
	w.WriteHeader(http.StatusNoContent)
}

// getAlertes recense les échéances dépassées ou proches.
func (s *Server) getAlertes(w http.ResponseWriter, r *http.Request) {
	alertes, err := s.st.Alertes()
	if err != nil {
		erreurStore(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, alertes)
}
