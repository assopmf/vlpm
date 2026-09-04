package store

import (
	"testing"
	"time"
)

func creerSession(t *testing.T, st *Store, userID int64, token, agent, ip string) {
	t.Helper()
	expire := time.Now().UTC().Add(12 * time.Hour).Format(time.RFC3339)
	if _, err := st.DB.Exec(`INSERT INTO sessions (token, user_id, expires_at, user_agent, ip)
		VALUES (?,?,?,?,?)`, token, userID, expire, agent, ip); err != nil {
		t.Fatal(err)
	}
}

func TestListeDesSessions(t *testing.T) {
	st := baseTest(t)
	creerSession(t, st, 1, "jeton-telephone", "Mozilla/5.0 (Linux; Android 14) Chrome/120", "10.0.0.5")
	creerSession(t, st, 1, "jeton-poste", "Mozilla/5.0 (Windows NT 10.0) Firefox/121", "10.0.0.9")

	sessions, err := st.SessionsDeLUtilisateur(1, "jeton-telephone")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("%d session(s), attendu 2", len(sessions))
	}

	var actuelles int
	for _, s := range sessions {
		if s.Actuelle {
			actuelles++
			if s.Appareil != "Android — Chrome" {
				t.Errorf("appareil courant = %q", s.Appareil)
			}
		}
	}
	if actuelles != 1 {
		t.Errorf("%d session(s) marquée(s) comme courante, attendu 1", actuelles)
	}
}

// Une session expirée ne doit pas figurer dans la liste : l'agent croirait
// qu'un appareil est encore connecté.
func TestSessionsExpireesExclues(t *testing.T) {
	st := baseTest(t)
	passe := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	st.DB.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES ('vieux', 1, ?)`, passe)
	creerSession(t, st, 1, "valide", "", "")

	sessions, _ := st.SessionsDeLUtilisateur(1, "valide")
	if len(sessions) != 1 {
		t.Errorf("%d session(s), attendu 1 : l'expirée devait être écartée", len(sessions))
	}
}

// Le point critique : personne ne doit pouvoir déconnecter quelqu'un d'autre
// en devinant un identifiant de ligne.
func TestRevocationLimiteeAuProprietaire(t *testing.T) {
	st := baseTest(t)
	autre := &User{Matricule: "1207", Nom: "Autre", Prenom: "Agent",
		PasswordHash: "x", Role: "agent", Actif: true}
	if err := st.CreateUser(autre); err != nil {
		t.Fatal(err)
	}
	creerSession(t, st, autre.ID, "jeton-de-l-autre", "", "")

	sessions, _ := st.SessionsDeLUtilisateur(autre.ID, "")
	if len(sessions) != 1 {
		t.Fatal("préparation du test incorrecte")
	}
	cible := sessions[0].ID

	// L'agent 1 tente de couper la session de l'agent 2.
	if err := st.RevoquerSession(1, cible); err == nil {
		t.Fatal("un agent a pu révoquer la session d'un autre")
	}
	// Le propriétaire, lui, y arrive.
	if err := st.RevoquerSession(autre.ID, cible); err != nil {
		t.Fatalf("le propriétaire ne peut pas révoquer sa propre session : %v", err)
	}
}

func TestRevoquerLesAutresConserveLaCourante(t *testing.T) {
	st := baseTest(t)
	creerSession(t, st, 1, "courante", "", "")
	creerSession(t, st, 1, "telephone-perdu", "", "")
	creerSession(t, st, 1, "poste-partage", "", "")

	n, err := st.RevoquerAutresSessions(1, "courante")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("%d session(s) révoquée(s), attendu 2", n)
	}
	sessions, _ := st.SessionsDeLUtilisateur(1, "courante")
	if len(sessions) != 1 || !sessions[0].Actuelle {
		t.Error("la session courante aurait dû être conservée")
	}
}

func TestDescriptionAppareil(t *testing.T) {
	cas := map[string]string{
		"Mozilla/5.0 (Linux; Android 14; SM-A536B) AppleWebKit Chrome/120 Mobile Safari": "Android — Chrome",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2) AppleWebKit Version/17.2 Safari":       "iPhone — Safari",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit Chrome/120 Safari Edg/120":            "Windows — Edge",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) Firefox/121":                      "Mac — Firefox",
		// L'application mobile annonce aussi son système : « Android —
		// application VLPM » renseigne mieux qu'une mention isolée.
		"VLPM-Android/1.0 (Android 14)": "Android — application VLPM",
		"VLPM/1.0":                      "application VLPM",
		"":                              "Appareil inconnu",
	}
	for agent, attendu := range cas {
		if got := DecrireAppareil(agent); got != attendu {
			t.Errorf("DecrireAppareil(%.40q) = %q, attendu %q", agent, got, attendu)
		}
	}
}
