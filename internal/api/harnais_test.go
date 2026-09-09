package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assopmf/vlpm/internal/auth"
	"github.com/assopmf/vlpm/internal/config"
	"github.com/assopmf/vlpm/internal/store"
)

// harnais monte un serveur complet sur une base temporaire, avec un compte de
// chaque rôle. Les tests portent sur le comportement observable de l'API :
// c'est ce qu'un contributeur casse sans s'en apercevoir.
type harnais struct {
	t       *testing.T
	st      *store.Store
	handler http.Handler
	jetons  map[string]string // rôle -> jeton de session
	comptes map[string]*store.User
}

func nouveauHarnais(t *testing.T) *harnais {
	t.Helper()
	dossier := t.TempDir()

	st, err := store.Open(filepath.Join(dossier, "test.db"))
	if err != nil {
		t.Fatalf("ouverture de la base : %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config.Config{DataDir: dossier, Addr: "127.0.0.1:0"}
	authSvc := auth.New(st)
	// Journal muet : les tests ne doivent pas noyer la sortie.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(cfg, st, authSvc, log, nil)

	h := &harnais{
		t: t, st: st, handler: srv.Handler(),
		jetons: map[string]string{}, comptes: map[string]*store.User{},
	}

	for _, role := range []string{"agent", "chef", "admin"} {
		h.creerCompte(role, role+"1", "MotDePasse123")
	}
	// Un véhicule et une sortie, pour que les routes portant un identifiant
	// aient une ressource existante à viser.
	v := &store.Vehicle{Code: "TV1", Statut: "disponible", KM: 1000, QRToken: "jeton-tv1"}
	if err := st.CreateVehicle(v); err != nil {
		t.Fatalf("création du véhicule : %v", err)
	}
	return h
}

func (h *harnais) creerCompte(role, matricule, motDePasse string) *store.User {
	h.t.Helper()
	hash, err := auth.HashPassword(motDePasse)
	if err != nil {
		h.t.Fatal(err)
	}
	u := &store.User{
		Matricule: matricule, Nom: strings.ToUpper(role), Prenom: "Test",
		Email: matricule + "@exemple.fr", PasswordHash: hash,
		Role: role, Actif: true,
	}
	if err := h.st.CreateUser(u); err != nil {
		h.t.Fatalf("création du compte %s : %v", matricule, err)
	}
	h.comptes[role] = u

	_, jeton, err := auth.New(h.st).Login(matricule, motDePasse, "", "test-agent", "127.0.0.1")
	if err != nil {
		h.t.Fatalf("connexion de %s : %v", matricule, err)
	}
	h.jetons[role] = jeton
	return u
}

// appel exécute une requête. Un rôle vide signifie « sans authentification ».
func (h *harnais) appel(methode, chemin, role, corps string) *httptest.ResponseRecorder {
	h.t.Helper()
	var lecteur io.Reader
	if corps != "" {
		lecteur = strings.NewReader(corps)
	}
	req := httptest.NewRequest(methode, chemin, lecteur)
	if corps != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		if jeton, ok := h.jetons[role]; ok {
			req.Header.Set("Authorization", "Bearer "+jeton)
		} else {
			req.Header.Set("Authorization", "Bearer "+role) // jeton brut
		}
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

// codeErreur extrait le code métier d'une réponse d'erreur.
func codeErreur(rec *httptest.ResponseRecorder) string {
	var r struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	return r.Code
}

func decoder(t *testing.T, rec *httptest.ResponseRecorder, cible any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), cible); err != nil {
		t.Fatalf("réponse illisible (%s) : %v", rec.Body.String(), err)
	}
}

// corpsMultipart compose un envoi de fichier, pour tester le téléversement
// des photos sans dépendre d'un vrai client.
func corpsMultipart(champ, nom, typeDeclare, contenu string) (string, string) {
	limite := "----vlpmtest"
	var b strings.Builder
	fmt.Fprintf(&b, "--%s\r\n", limite)
	fmt.Fprintf(&b, "Content-Disposition: form-data; name=%q; filename=%q\r\n", champ, nom)
	fmt.Fprintf(&b, "Content-Type: %s\r\n\r\n", typeDeclare)
	b.WriteString(contenu)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", limite)
	return b.String(), "multipart/form-data; boundary=" + limite
}

// appelBrut permet de fixer soi-même le type de contenu.
func (h *harnais) appelBrut(methode, chemin, role, corps, typeContenu string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(methode, chemin, strings.NewReader(corps))
	req.Header.Set("Content-Type", typeContenu)
	if jeton, ok := h.jetons[role]; ok {
		req.Header.Set("Authorization", "Bearer "+jeton)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}
