// Package api expose l'API REST et sert l'interface web embarquée.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/assopmf/vlpm/internal/auth"
	"github.com/assopmf/vlpm/internal/config"
	"github.com/assopmf/vlpm/internal/store"
)

type Server struct {
	cfg    config.Config
	st     *store.Store
	auth   *auth.Service
	log    *slog.Logger
	limite *limiteur
}

func New(cfg config.Config, st *store.Store, a *auth.Service, log *slog.Logger) *Server {
	return &Server{cfg: cfg, st: st, auth: a, log: log, limite: nouveauLimiteur()}
}

// contexte de requête : l'utilisateur authentifié est passé via le contexte.
type cleContexte string

const cleUser cleContexte = "vlpm.user"

func utilisateurDe(r *http.Request) *store.User {
	u, _ := r.Context().Value(cleUser).(*store.User)
	return u
}

// --- Réponses JSON ---

type reponseErreur struct {
	Erreur string `json:"erreur"`
	Code   string `json:"code,omitempty"`
}

func ecrireJSON(w http.ResponseWriter, statut int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statut)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// La réponse est déjà partiellement écrite : on ne peut que journaliser.
		slog.Error("écriture de la réponse JSON", "erreur", err)
	}
}

func erreur(w http.ResponseWriter, statut int, message string, codes ...string) {
	code := ""
	if len(codes) > 0 {
		code = codes[0]
	}
	ecrireJSON(w, statut, reponseErreur{Erreur: message, Code: code})
}

// erreurStore traduit les erreurs métier en codes HTTP appropriés.
func erreurStore(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		erreur(w, http.StatusNotFound, "Ressource introuvable.", "introuvable")
	case errors.Is(err, store.ErrAgentDejaDetenteur):
		erreur(w, http.StatusConflict,
			majuscule(err.Error())+". Il doit le restituer avant d'en prendre un autre.", "deja_detenteur")
	case errors.Is(err, store.ErrDejaEnService):
		erreur(w, http.StatusConflict, "Ce véhicule est déjà pris en compte par un autre agent.", "deja_en_service")
	case errors.Is(err, store.ErrVehiculeIndisponible):
		erreur(w, http.StatusConflict, err.Error(), "indisponible")
	case errors.Is(err, store.ErrHorodatageInvalide):
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "horodatage_invalide")
	case errors.Is(err, store.ErrKMIncoherent):
		erreur(w, http.StatusUnprocessableEntity, majuscule(err.Error()), "km_incoherent")
	case estContrainteUnique(err):
		erreur(w, http.StatusConflict, "Cette valeur est déjà utilisée (code véhicule ou matricule en double).", "doublon")
	default:
		slog.Error("erreur interne", "erreur", err)
		erreur(w, http.StatusInternalServerError, "Erreur interne du serveur.", "interne")
	}
}

func estContrainteUnique(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func majuscule(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func decoderJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	// 1 Mio suffit largement et protège contre un corps de requête abusif.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		erreur(w, http.StatusBadRequest, "Requête JSON invalide : "+err.Error(), "json_invalide")
		return false
	}
	return true
}

// --- Middlewares ---

// authentifier exige un jeton de session valide.
func (s *Server) authentifier(suivant http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := jetonDe(r)
		u, err := s.auth.Utilisateur(token)
		if err != nil {
			erreur(w, http.StatusUnauthorized, "Authentification requise.", "non_authentifie")
			return
		}
		suivant(w, r.WithContext(context.WithValue(r.Context(), cleUser, u)))
	}
}

// exiger compose authentifier avec un niveau de rôle minimum.
func (s *Server) exiger(role string, suivant http.HandlerFunc) http.HandlerFunc {
	return s.authentifier(func(w http.ResponseWriter, r *http.Request) {
		if u := utilisateurDe(r); u == nil || !u.Peut(role) {
			erreur(w, http.StatusForbidden, "Vous n'avez pas les droits nécessaires pour cette action.", "droits_insuffisants")
			return
		}
		suivant(w, r)
	})
}

// jetonDe extrait le jeton de l'en-tête Authorization: Bearer.
// L'application web utilise le même mécanisme que la future application Android,
// ce qui écarte au passage tout risque de CSRF (aucun cookie d'authentification).
func jetonDe(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func (s *Server) journaliser(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		debut := time.Now()
		rec := &enregistreur{ResponseWriter: w, statut: http.StatusOK}
		suivant.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.log.Info("requête",
				"methode", r.Method, "chemin", r.URL.Path,
				"statut", rec.statut, "duree_ms", time.Since(debut).Milliseconds(),
				"ip", s.ipDe(r))
		}
	})
}

func enTetesSecurite(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		// L'interface est entièrement locale : aucune ressource externe autorisée.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; "+
				"style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; "+
				"worker-src 'self'; manifest-src 'self'; "+
				"frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("Permissions-Policy", "camera=(self), geolocation=(), microphone=()")
		suivant.ServeHTTP(w, r)
	})
}

type enregistreur struct {
	http.ResponseWriter
	statut int
}

func (e *enregistreur) WriteHeader(code int) {
	e.statut = code
	e.ResponseWriter.WriteHeader(code)
}

// ipDe renvoie l'IP cliente, en tenant compte du reverse proxy si configuré.
func (s *Server) ipDe(r *http.Request) string {
	if s.cfg.DerriereProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- Limiteur de tentatives de connexion ---

// limiteur bloque le bourrage d'identifiants : 10 tentatives par IP et par
// tranche de 15 minutes. En mémoire, donc remis à zéro au redémarrage : c'est
// suffisant pour une instance communale.
type limiteur struct {
	mu         sync.Mutex
	tentatives map[string][]time.Time
}

const (
	limiteFenetre = 15 * time.Minute
	limiteMax     = 10
)

func nouveauLimiteur() *limiteur {
	return &limiteur{tentatives: map[string][]time.Time{}}
}

func (l *limiteur) autorise(cle string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	maintenant := time.Now()
	recentes := l.tentatives[cle][:0]
	for _, t := range l.tentatives[cle] {
		if maintenant.Sub(t) < limiteFenetre {
			recentes = append(recentes, t)
		}
	}
	if len(recentes) >= limiteMax {
		l.tentatives[cle] = recentes
		return false
	}
	l.tentatives[cle] = append(recentes, maintenant)
	return true
}

func (l *limiteur) reussite(cle string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tentatives, cle)
}

// purger évite que la table grossisse indéfiniment sur une instance exposée.
func (l *limiteur) purger() {
	l.mu.Lock()
	defer l.mu.Unlock()
	maintenant := time.Now()
	for cle, ts := range l.tentatives {
		garde := ts[:0]
		for _, t := range ts {
			if maintenant.Sub(t) < limiteFenetre {
				garde = append(garde, t)
			}
		}
		if len(garde) == 0 {
			delete(l.tentatives, cle)
		} else {
			l.tentatives[cle] = garde
		}
	}
}

// --- Utilitaires de paramètres ---

func idPath(r *http.Request, nom string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(nom), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func queryInt(r *http.Request, nom string, def int) int {
	if v := r.URL.Query().Get(nom); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// PurgerLimiteur libère la mémoire des tentatives de connexion périmées.
func (s *Server) PurgerLimiteur() { s.limite.purger() }
