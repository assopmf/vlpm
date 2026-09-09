package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Ces tests reprennent les vérifications faites à la main pendant le
// développement. Elles fonctionnaient, mais ne laissaient aucune trace
// exécutable : un contributeur pouvait les casser sans rien voir devenir rouge.

// --- Cloisonnement des données ---

// Un agent ne doit voir que ses propres sorties, y compris s'il demande
// explicitement celles d'un collègue.
func TestAgentNeVoitQueSesSorties(t *testing.T) {
	h := nouveauHarnais(t)

	// Le chef confie le véhicule à l'agent, puis en crée une pour lui-même.
	h.appel("POST", "/api/v1/prises", "chef",
		fmt.Sprintf(`{"vehicule_id":1,"km":1000,"agent_id":%d}`, h.comptes["agent"].ID))

	rec := h.appel("GET", "/api/v1/prises?agent=2", "agent", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body.String())
	}
	var sorties []map[string]any
	decoder(t, rec, &sorties)
	for _, s := range sorties {
		if int64(s["user_id"].(float64)) != h.comptes["agent"].ID {
			t.Errorf("un agent voit la sortie de quelqu'un d'autre : %v", s)
		}
	}
}

// --- Saisie au nom d'un tiers ---

func TestSeulUnChefPeutSaisirPourUnAutre(t *testing.T) {
	h := nouveauHarnais(t)
	cible := h.comptes["chef"].ID

	rec := h.appel("POST", "/api/v1/prises", "agent",
		fmt.Sprintf(`{"vehicule_id":1,"km":1000,"agent_id":%d}`, cible))
	if rec.Code != http.StatusForbidden {
		t.Errorf("un agent a pu saisir au nom d'un autre : HTTP %d (%s)",
			rec.Code, rec.Body.String())
	}
	if c := codeErreur(rec); c != "droits_insuffisants" {
		t.Errorf("code = %q", c)
	}
}

func TestSaisieDelegueeAttribueAuBonAgent(t *testing.T) {
	h := nouveauHarnais(t)
	agent := h.comptes["agent"]

	rec := h.appel("POST", "/api/v1/prises", "chef",
		fmt.Sprintf(`{"vehicule_id":1,"km":1000,"agent_id":%d}`, agent.ID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body.String())
	}
	var c map[string]any
	decoder(t, rec, &c)
	if int64(c["user_id"].(float64)) != agent.ID {
		t.Errorf("détenteur = %v, attendu l'agent %d", c["user_id"], agent.ID)
	}
	// Le chef ne doit pas apparaître comme détenteur.
	rec = h.appel("GET", "/api/v1/moi", "chef", "")
	if strings.Contains(rec.Body.String(), "checkout_en_cours") {
		t.Error("le chef apparaît comme détenteur alors qu'il a saisi pour autrui")
	}
}

// --- Garde-fous métier ---

func TestKilometrageAberrantRefuse(t *testing.T) {
	h := nouveauHarnais(t)
	h.appel("POST", "/api/v1/prises", "agent", `{"vehicule_id":1,"km":1000}`)

	rec := h.appel("POST", "/api/v1/prises/1/restitution", "agent", `{"km":50000}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("HTTP %d, attendu 422 : %s", rec.Code, rec.Body.String())
	}
	if c := codeErreur(rec); c != "km_incoherent" {
		t.Errorf("code = %q, attendu km_incoherent", c)
	}
	// Avec confirmation, la même valeur doit passer.
	rec = h.appel("POST", "/api/v1/prises/1/restitution", "agent", `{"km":50000,"force":true}`)
	if rec.Code != http.StatusOK {
		t.Errorf("restitution forcée refusée : HTTP %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDesactiverUnAgentDetenteurRefuse(t *testing.T) {
	h := nouveauHarnais(t)
	agent := h.comptes["agent"]
	h.appel("POST", "/api/v1/prises", "agent", `{"vehicule_id":1,"km":1000}`)

	rec := h.appel("PATCH", fmt.Sprintf("/api/v1/agents/%d", agent.ID), "chef", `{"actif":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("HTTP %d, attendu 409 : %s", rec.Code, rec.Body.String())
	}
	if c := codeErreur(rec); c != "detient_vehicule" {
		t.Errorf("code = %q, attendu detient_vehicule", c)
	}
}

// Le rejeu d'une opération hors ligne doit renvoyer un succès, jamais un
// conflit : sinon le client réessaie indéfiniment une saisie déjà enregistrée.
func TestRejeuHorsLigneRenvoieUnSucces(t *testing.T) {
	h := nouveauHarnais(t)
	corps := `{"vehicule_id":1,"km":1000,"cle_client":"cle-test"}`

	if rec := h.appel("POST", "/api/v1/prises", "agent", corps); rec.Code != http.StatusCreated {
		t.Fatalf("première prise : HTTP %d (%s)", rec.Code, rec.Body.String())
	}
	for i := 0; i < 3; i++ {
		rec := h.appel("POST", "/api/v1/prises", "agent", corps)
		if rec.Code != http.StatusOK {
			t.Fatalf("rejeu %d : HTTP %d, attendu 200 (%s)", i+1, rec.Code, rec.Body.String())
		}
	}
	var sorties []map[string]any
	decoder(t, h.appel("GET", "/api/v1/prises", "agent", ""), &sorties)
	if len(sorties) != 1 {
		t.Errorf("%d sorties enregistrées, attendu 1 malgré les rejeux", len(sorties))
	}
}

// --- Sessions ---

// Un identifiant deviné ne doit pas permettre de déconnecter un tiers.
func TestRevocationDeSessionLimiteeAuProprietaire(t *testing.T) {
	h := nouveauHarnais(t)

	var sessions []map[string]any
	decoder(t, h.appel("GET", "/api/v1/moi/sessions", "chef", ""), &sessions)
	if len(sessions) == 0 {
		t.Fatal("aucune session pour le chef")
	}
	idChef := int64(sessions[0]["id"].(float64))

	rec := h.appel("DELETE", fmt.Sprintf("/api/v1/moi/sessions/%d", idChef), "agent", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("un agent a pu viser la session du chef : HTTP %d", rec.Code)
	}
	// Le chef reste connecté.
	if rec := h.appel("GET", "/api/v1/moi", "chef", ""); rec.Code != http.StatusOK {
		t.Error("la session du chef a été coupée par un tiers")
	}
}

// --- Photos ---

// Le format est déterminé en lisant l'en-tête du fichier : un contenu qui
// n'est pas une image doit être refusé même déclaré image/jpeg.
func TestPhotoNonImageRefusee(t *testing.T) {
	h := nouveauHarnais(t)
	h.appel("POST", "/api/v1/incidents", "agent", `{"vehicule_id":1,"description":"x"}`)

	corps, typeMIME := corpsMultipart("photo", "innocent.jpg", "image/jpeg",
		"#!/bin/sh\nrm -rf /\n")
	rec := h.appelBrut("POST", "/api/v1/incidents/1/photos", "agent", corps, typeMIME)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("HTTP %d, attendu 422 : %s", rec.Code, rec.Body.String())
	}
	if c := codeErreur(rec); c != "format_invalide" {
		t.Errorf("code = %q, attendu format_invalide", c)
	}
}

// --- Connexion ---

func TestBlocageApresEchecsRepetes(t *testing.T) {
	h := nouveauHarnais(t)
	for i := 0; i < 6; i++ {
		h.appel("POST", "/api/v1/auth/login", "",
			`{"matricule":"agent1","mot_de_passe":"faux"}`)
	}
	// Même le bon mot de passe doit être refusé pendant le blocage.
	rec := h.appel("POST", "/api/v1/auth/login", "",
		`{"matricule":"agent1","mot_de_passe":"MotDePasse123"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("HTTP %d, attendu 429 : %s", rec.Code, rec.Body.String())
	}
	// Un autre compte ne doit pas être affecté.
	rec = h.appel("POST", "/api/v1/auth/login", "",
		`{"matricule":"chef1","mot_de_passe":"MotDePasse123"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("le blocage déborde sur un autre compte : HTTP %d", rec.Code)
	}
}

// La réponse ne doit pas révéler qu'un matricule existe.
func TestPasDEnumerationDesComptes(t *testing.T) {
	h := nouveauHarnais(t)
	existant := h.appel("POST", "/api/v1/auth/login", "",
		`{"matricule":"agent1","mot_de_passe":"faux"}`)
	inconnu := h.appel("POST", "/api/v1/auth/login", "",
		`{"matricule":"nexistepas","mot_de_passe":"faux"}`)

	if existant.Code != inconnu.Code {
		t.Errorf("codes différents : %d pour un compte existant, %d pour un inconnu",
			existant.Code, inconnu.Code)
	}
	if existant.Body.String() != inconnu.Body.String() {
		t.Errorf("réponses différentes :\n  existant : %s\n  inconnu  : %s",
			existant.Body.String(), inconnu.Body.String())
	}
}
