package api

import (
	"net/http"
	"strings"
	"testing"
)

// route décrit une entrée de l'API et le rôle minimal qu'elle exige.
type route struct {
	methode string
	chemin  string
	role    string // "agent", "chef" ou "admin"
	corps   string // corps minimal, pour ne pas échouer avant le contrôle de rôle
}

// toutesLesRoutes recense les routes protégées.
//
// Cette table est le cœur du test : elle transcrit ce que routes.go déclare.
// Un contributeur qui abaisse par mégarde la protection d'une route la verra
// échouer ici, alors qu'aucun test fonctionnel ne l'aurait remarqué.
var toutesLesRoutes = []route{
	{"GET", "/api/v1/version", "agent", ""},
	{"POST", "/api/v1/auth/logout", "agent", ""},
	{"GET", "/api/v1/moi", "agent", ""},
	{"POST", "/api/v1/moi/mot-de-passe", "agent", `{}`},
	{"GET", "/api/v1/moi/sessions", "agent", ""},
	{"DELETE", "/api/v1/moi/sessions/1", "agent", ""},
	{"POST", "/api/v1/moi/sessions/revoquer-autres", "agent", ""},
	{"GET", "/api/v1/moi/totp", "agent", ""},
	{"POST", "/api/v1/moi/totp/preparer", "agent", ""},
	{"GET", "/api/v1/moi/totp/qrcode.png", "agent", ""},
	{"POST", "/api/v1/moi/totp/activer", "agent", `{}`},
	{"DELETE", "/api/v1/moi/totp", "agent", `{}`},

	{"GET", "/api/v1/vehicules", "agent", ""},
	{"GET", "/api/v1/vehicules/1", "agent", ""},
	{"GET", "/api/v1/scan/jeton-tv1", "agent", ""},
	{"GET", "/api/v1/prises", "agent", ""},
	{"POST", "/api/v1/prises", "agent", `{"vehicule_id":1,"km":1000}`},
	{"POST", "/api/v1/prises/1/restitution", "agent", `{"km":1000}`},
	{"GET", "/api/v1/incidents", "agent", ""},
	{"POST", "/api/v1/incidents", "agent", `{"vehicule_id":1,"description":"x"}`},
	{"POST", "/api/v1/incidents/1/photos", "agent", ""},
	{"GET", "/api/v1/photos/1", "agent", ""},
	{"GET", "/api/v1/alertes", "agent", ""},
	{"GET", "/api/v1/entretiens", "agent", ""},
	{"GET", "/api/v1/stats", "agent", ""},
	{"GET", "/api/v1/agents", "agent", ""},

	{"POST", "/api/v1/vehicules", "chef", `{"code":"TV9"}`},
	{"PATCH", "/api/v1/vehicules/1", "chef", `{"marque":"X"}`},
	{"GET", "/api/v1/vehicules/1/qrcode.png", "chef", ""},
	{"POST", "/api/v1/incidents/1/resolution", "chef", ""},
	{"DELETE", "/api/v1/photos/1", "chef", ""},
	{"POST", "/api/v1/entretiens", "chef", `{"vehicule_id":1}`},
	{"GET", "/api/v1/export/prises.csv", "chef", ""},
	{"GET", "/api/v1/export/parc.csv", "chef", ""},
	{"GET", "/api/v1/export/incidents.csv", "chef", ""},
	{"GET", "/api/v1/export/entretiens.csv", "chef", ""},
	{"GET", "/api/v1/notifications", "chef", ""},
	{"PATCH", "/api/v1/notifications", "chef", `{"frequence":"desactivee"}`},
	{"POST", "/api/v1/notifications/envoyer", "chef", ""},
	{"POST", "/api/v1/agents", "chef", `{"matricule":"9999","nom":"X"}`},
	{"PATCH", "/api/v1/agents/1", "chef", `{"nom":"X"}`},
	{"POST", "/api/v1/agents/1/mot-de-passe", "chef", ""},

	{"GET", "/api/v1/securite/totp", "admin", ""},
	{"PATCH", "/api/v1/securite/totp", "admin", `{"mode":"desactive"}`},
	{"POST", "/api/v1/agents/1/totp/reinitialiser", "admin", ""},
	{"GET", "/api/v1/journal", "admin", ""},
	{"GET", "/api/v1/conservation", "admin", ""},
	{"PATCH", "/api/v1/conservation", "admin", `{"activite_mois":0}`},
	{"POST", "/api/v1/conservation/simulation", "admin", `{}`},
	{"POST", "/api/v1/conservation/purger", "admin", ""},
}

// Aucune route ne doit répondre sans jeton valide. C'est la garantie que le
// premier venu connaissant l'adresse de l'API n'obtient rien.
func TestAucuneRouteSansAuthentification(t *testing.T) {
	h := nouveauHarnais(t)
	for _, r := range toutesLesRoutes {
		t.Run(r.methode+" "+r.chemin, func(t *testing.T) {
			rec := h.appel(r.methode, r.chemin, "", r.corps)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("HTTP %d sans jeton, attendu 401 (corps : %.120s)",
					rec.Code, rec.Body.String())
			}
			if c := codeErreur(rec); c != "non_authentifie" {
				t.Errorf("code = %q, attendu non_authentifie", c)
			}
		})
	}
}

// Un jeton inventé ne doit pas davantage ouvrir l'API.
func TestJetonInvalideRejete(t *testing.T) {
	h := nouveauHarnais(t)
	for _, jeton := range []string{"nimportequoi", "", "Bearer", strings.Repeat("a", 300)} {
		rec := h.appel("GET", "/api/v1/vehicules", jeton, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("jeton %.20q accepté : HTTP %d", jeton, rec.Code)
		}
	}
}

// Le cloisonnement proprement dit : un rôle insuffisant doit être refusé, un
// rôle suffisant ne doit jamais l'être.
//
// L'assertion porte sur le seul contrôle de rôle, pas sur le résultat métier :
// une route peut légitimement répondre 404 ou 422 selon le corps envoyé, mais
// jamais 403 pour qui en a le droit.
func TestCloisonnementDesRoles(t *testing.T) {
	rang := map[string]int{"agent": 1, "chef": 2, "admin": 3}

	for _, r := range toutesLesRoutes {
		for _, role := range []string{"agent", "chef", "admin"} {
			nom := r.methode + " " + r.chemin + " en " + role
			t.Run(nom, func(t *testing.T) {
				// Base neuve par cas : certaines routes modifient l'état
				// (restitution, purge) et fausseraient les suivantes.
				h := nouveauHarnais(t)
				rec := h.appel(r.methode, r.chemin, role, r.corps)
				suffisant := rang[role] >= rang[r.role]

				if suffisant && rec.Code == http.StatusForbidden {
					t.Errorf("%s refusé à un %s, qui devrait y avoir droit (%s)",
						r.chemin, role, rec.Body.String())
				}
				if !suffisant && rec.Code != http.StatusForbidden {
					t.Errorf("%s accessible à un %s : HTTP %d, attendu 403 (%.120s)",
						r.chemin, role, rec.Code, rec.Body.String())
				}
			})
		}
	}
}
