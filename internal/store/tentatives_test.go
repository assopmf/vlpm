package store

import (
	"testing"
	"time"
)

func TestBlocageParIP(t *testing.T) {
	st := baseTest(t)
	for i := 0; i < MaxParIP-1; i++ {
		// Des matricules différents : seule l'adresse est commune.
		if err := st.EnregistrerEchec("agent"+string(rune('a'+i)), "203.0.113.7"); err != nil {
			t.Fatal(err)
		}
	}
	b, err := st.ConnexionBloquee("encore-un-autre", "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	if b.Bloque {
		t.Fatalf("bloqué à %d tentatives, la limite est %d", MaxParIP-1, MaxParIP)
	}

	st.EnregistrerEchec("dernier", "203.0.113.7")
	b, _ = st.ConnexionBloquee("peu-importe", "203.0.113.7")
	if !b.Bloque || b.Cause != "ip" {
		t.Fatalf("blocage = %+v, attendu un blocage par IP", b)
	}
	if b.Restant <= 0 || b.Restant > FenetreTentatives {
		t.Errorf("délai restant = %v, attendu entre 0 et %v", b.Restant, FenetreTentatives)
	}
}

// Le cas que la seule limite par IP laissait passer : un même compte attaqué
// depuis plusieurs adresses.
func TestBlocageParMatriculeMalgreDesIPDifferentes(t *testing.T) {
	st := baseTest(t)
	for i := 0; i < MaxParMatricule; i++ {
		ip := "198.51.100." + string(rune('1'+i))
		if err := st.EnregistrerEchec("1204", ip); err != nil {
			t.Fatal(err)
		}
	}
	// Une adresse encore jamais vue : elle doit tout de même être refusée.
	b, err := st.ConnexionBloquee("1204", "203.0.113.99")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Bloque || b.Cause != "matricule" {
		t.Fatalf("blocage = %+v, attendu un blocage par matricule", b)
	}
	// Un autre compte depuis cette même adresse reste accessible : le blocage
	// ne doit pas déborder sur les collègues.
	if b, _ := st.ConnexionBloquee("1207", "203.0.113.99"); b.Bloque {
		t.Error("le blocage d'un compte empêche la connexion d'un autre")
	}
}

func TestMatriculeInsensibleALaCasse(t *testing.T) {
	st := baseTest(t)
	for i := 0; i < MaxParMatricule; i++ {
		st.EnregistrerEchec("Admin", "198.51.100.1")
	}
	if b, _ := st.ConnexionBloquee("admin", "203.0.113.50"); !b.Bloque {
		t.Error("« admin » et « Admin » devraient compter pour le même compte")
	}
}

// Une connexion réussie doit solder le compteur : un agent qui se trompe deux
// fois puis réussit ne doit pas rester à deux.
func TestReussiteEffaceLesEchecs(t *testing.T) {
	st := baseTest(t)
	for i := 0; i < 3; i++ {
		st.EnregistrerEchec("1204", "198.51.100.1")
	}
	if err := st.EffacerEchecs("1204", "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxParMatricule-1; i++ {
		st.EnregistrerEchec("1204", "198.51.100.1")
	}
	if b, _ := st.ConnexionBloquee("1204", "198.51.100.1"); b.Bloque {
		t.Error("le compteur n'a pas été soldé par la connexion réussie")
	}
}

// Le blocage se lève tout seul : un verrouillage définitif permettrait de
// paralyser un service entier avant une prise de service.
func TestBlocageSeLeveApresLaFenetre(t *testing.T) {
	st := baseTest(t)
	vieux := time.Now().UTC().Add(-FenetreTentatives - time.Minute).Format(time.RFC3339)
	for i := 0; i < MaxParMatricule+2; i++ {
		if _, err := st.DB.Exec(`INSERT INTO tentatives_connexion (matricule, ip, created_at)
			VALUES ('1204', '198.51.100.1', ?)`, vieux); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := st.ConnexionBloquee("1204", "198.51.100.1"); b.Bloque {
		t.Error("des tentatives sorties de la fenêtre bloquent encore la connexion")
	}
}

func TestPurgeDesTentatives(t *testing.T) {
	st := baseTest(t)
	vieux := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	st.DB.Exec(`INSERT INTO tentatives_connexion (matricule, ip, created_at) VALUES ('a','1.2.3.4',?)`, vieux)
	st.EnregistrerEchec("b", "1.2.3.4")

	n, err := st.PurgerTentatives()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d ligne(s) purgée(s), attendu 1", n)
	}
	var reste int
	st.DB.QueryRow(`SELECT COUNT(*) FROM tentatives_connexion`).Scan(&reste)
	if reste != 1 {
		t.Errorf("%d ligne(s) restante(s), la tentative récente devait être conservée", reste)
	}
}
