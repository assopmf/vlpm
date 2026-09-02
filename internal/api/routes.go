package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assopmf/vlpm/web"
)

// Handler construit l'arbre de routage complet : API REST puis interface web.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Public ---
	mux.HandleFunc("POST /api/v1/auth/login", s.postLogin)
	mux.HandleFunc("GET /api/v1/version", s.getVersion)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.st.DB.Ping(); err != nil {
			erreur(w, http.StatusServiceUnavailable, "Base de données injoignable.", "base_indisponible")
			return
		}
		ecrireJSON(w, http.StatusOK, map[string]string{"statut": "ok"})
	})

	// --- Session ---
	mux.HandleFunc("POST /api/v1/auth/logout", s.authentifier(s.postLogout))
	mux.HandleFunc("GET /api/v1/moi", s.authentifier(s.getMoi))
	mux.HandleFunc("POST /api/v1/moi/mot-de-passe", s.authentifier(s.postChangerMotDePasse))

	// --- Parc ---
	mux.HandleFunc("GET /api/v1/vehicules", s.authentifier(s.getVehicles))
	mux.HandleFunc("GET /api/v1/vehicules/{id}", s.authentifier(s.getVehicle))
	mux.HandleFunc("POST /api/v1/vehicules", s.exiger("chef", s.postVehicles))
	mux.HandleFunc("PATCH /api/v1/vehicules/{id}", s.exiger("chef", s.patchVehicle))
	mux.HandleFunc("GET /api/v1/vehicules/{id}/qrcode.png", s.exiger("chef", s.getQRCode))
	mux.HandleFunc("GET /api/v1/scan/{token}", s.authentifier(s.getScan))

	// --- Prises en compte ---
	mux.HandleFunc("GET /api/v1/prises", s.authentifier(s.getCheckouts))
	mux.HandleFunc("POST /api/v1/prises", s.authentifier(s.postPriseEnCompte))
	mux.HandleFunc("POST /api/v1/prises/{id}/restitution", s.authentifier(s.postRestitution))

	// --- Incidents et entretiens ---
	mux.HandleFunc("GET /api/v1/incidents", s.authentifier(s.getIncidents))
	mux.HandleFunc("POST /api/v1/incidents", s.authentifier(s.postIncidents))
	mux.HandleFunc("POST /api/v1/incidents/{id}/resolution", s.exiger("chef", s.postResoudreIncident))
	mux.HandleFunc("GET /api/v1/entretiens", s.authentifier(s.getMaintenances))
	mux.HandleFunc("POST /api/v1/entretiens", s.exiger("chef", s.postMaintenances))

	// --- Pilotage ---
	mux.HandleFunc("GET /api/v1/stats", s.authentifier(s.getStats))
	mux.HandleFunc("GET /api/v1/export/prises.csv", s.exiger("chef", s.getExportCSV))
	mux.HandleFunc("GET /api/v1/journal", s.exiger("admin", s.getAudit))

	// --- Comptes ---
	mux.HandleFunc("GET /api/v1/agents", s.authentifier(s.getUsers))
	mux.HandleFunc("POST /api/v1/agents", s.exiger("chef", s.postUsers))
	mux.HandleFunc("PATCH /api/v1/agents/{id}", s.exiger("chef", s.patchUser))
	mux.HandleFunc("POST /api/v1/agents/{id}/mot-de-passe", s.exiger("chef", s.postResetMotDePasse))

	// Toute route /api/ non reconnue doit répondre en JSON, pas en HTML.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		erreur(w, http.StatusNotFound, "Route d'API inconnue.", "route_inconnue")
	})

	mux.Handle("/", s.interfaceWeb())

	return enTetesSecurite(s.journaliser(mux))
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]string{
		"application": "VLPM",
		"version":     Version,
		"api":         "v1",
	})
}

// Version est renseignée à la compilation (-ldflags "-X ...Version=1.2.3").
var Version = "dev"

// interfaceWeb sert le front. En mode --dev les fichiers sont relus depuis le
// disque à chaque requête ; sinon ils viennent du binaire.
func (s *Server) interfaceWeb() http.Handler {
	var fsys fs.FS
	if s.cfg.Dev {
		racine := filepath.Join("web", "static")
		if _, err := os.Stat(racine); err == nil {
			s.log.Info("mode développement : interface servie depuis le disque", "dossier", racine)
			fsys = os.DirFS(racine)
		} else {
			s.log.Warn("mode développement demandé mais dossier introuvable, retour à l'interface embarquée", "dossier", racine)
		}
	}
	if fsys == nil {
		fsys = web.FS()
	}
	serveur := http.FileServer(http.FS(fsys))

	// Empreinte de l'ensemble des fichiers servis. Elle change dès qu'un octet
	// de l'interface change, donc à chaque nouvelle version du binaire.
	etag := empreinte(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chemin := strings.TrimPrefix(r.URL.Path, "/")
		if chemin == "" {
			chemin = "index.html"
		}
		if _, err := fs.Stat(fsys, chemin); err != nil {
			// Route applicative (/scan/xxx, /vehicule/3, ...) : l'interface est
			// une page unique qui gère elle-même sa navigation.
			servirIndex(w, r, fsys)
			return
		}

		// "no-cache" n'interdit pas la mise en cache : il impose de revalider.
		// Le navigateur renvoie son ETag, on répond 304 si rien n'a bougé. Une
		// mise à jour du serveur est donc prise en compte au rechargement
		// suivant, sans laisser les agents sur une interface périmée.
		if s.cfg.Dev {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		serveur.ServeHTTP(w, r)
	})
}

// empreinte calcule un ETag couvrant tous les fichiers de l'interface.
func empreinte(fsys fs.FS) string {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(chemin string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		contenu, err := fs.ReadFile(fsys, chemin)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s:%x\n", chemin, sha256.Sum256(contenu))
		return nil
	})
	if err != nil {
		// Sans empreinte fiable, mieux vaut ne jamais servir de cache périmé.
		return fmt.Sprintf("%q", time.Now().UnixNano())
	}
	return fmt.Sprintf("%q", hex.EncodeToString(h.Sum(nil))[:16])
}

func servirIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	f, err := fsys.Open("index.html")
	if err != nil {
		http.Error(w, "interface introuvable", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	contenu, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "interface illisible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(contenu)
}
