package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSauvegardeInstantaneUtilisable(t *testing.T) {
	st := baseTest(t)
	if _, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280}); err != nil {
		t.Fatal(err)
	}

	dossier := t.TempDir()
	dest := filepath.Join(dossier, NomSauvegarde(time.Now()))
	taille, err := st.Sauvegarder(dest)
	if err != nil {
		t.Fatalf("sauvegarde : %v", err)
	}
	if taille == 0 {
		t.Fatal("sauvegarde vide")
	}

	// La sauvegarde doit s'ouvrir seule et contenir les mêmes données : une
	// sauvegarde qu'on ne peut pas relire ne vaut rien.
	copie, err := Open(dest)
	if err != nil {
		t.Fatalf("réouverture de la sauvegarde : %v", err)
	}
	defer copie.Close()

	v, err := copie.VehicleByCode("TV1")
	if err != nil {
		t.Fatalf("véhicule absent de la sauvegarde : %v", err)
	}
	if v.Statut != "en_service" {
		t.Errorf("statut = %q, attendu en_service : la sortie en cours n'a pas été capturée", v.Statut)
	}
	if _, err := copie.CheckoutEnCours(1); err != nil {
		t.Errorf("la prise en compte devrait figurer dans la sauvegarde : %v", err)
	}
}

// La sauvegarde contient les mêmes données nominatives que la base : elle ne
// doit pas être lisible par les autres comptes de la machine.
func TestSauvegardePermissionsRestreintes(t *testing.T) {
	st := baseTest(t)
	dest := filepath.Join(t.TempDir(), NomSauvegarde(time.Now()))
	if _, err := st.Sauvegarder(dest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("permissions = %o, attendu 600", mode)
	}
}

func TestSauvegardeRefuseDEcraser(t *testing.T) {
	st := baseTest(t)
	dest := filepath.Join(t.TempDir(), "deja-la.db")
	if err := os.WriteFile(dest, []byte("contenu existant"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sauvegarder(dest); err == nil {
		t.Fatal("écraser une sauvegarde existante devrait être refusé")
	}
	// Le fichier d'origine doit être intact.
	contenu, _ := os.ReadFile(dest)
	if string(contenu) != "contenu existant" {
		t.Error("le fichier existant a été altéré")
	}
}

func TestPurgeConserveLesPlusRecentes(t *testing.T) {
	dossier := t.TempDir()
	// Les noms horodatés se trient alphabétiquement dans l'ordre chronologique.
	noms := []string{
		"vlpm-2026-09-01-030000.db", "vlpm-2026-09-02-030000.db",
		"vlpm-2026-09-03-030000.db", "vlpm-2026-09-04-030000.db",
		"vlpm-2026-09-05-030000.db",
	}
	for _, n := range noms {
		if err := os.WriteFile(filepath.Join(dossier, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Un fichier étranger au dossier ne doit pas être touché.
	autre := filepath.Join(dossier, "notes-du-service.txt")
	if err := os.WriteFile(autre, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	supprimes, err := PurgerSauvegardes(dossier, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(supprimes) != 3 {
		t.Fatalf("%d fichiers supprimés, attendu 3", len(supprimes))
	}

	restants, _ := os.ReadDir(dossier)
	var db []string
	for _, e := range restants {
		if strings.HasSuffix(e.Name(), ".db") {
			db = append(db, e.Name())
		}
	}
	if len(db) != 2 || db[0] != "vlpm-2026-09-04-030000.db" || db[1] != "vlpm-2026-09-05-030000.db" {
		t.Errorf("sauvegardes conservées = %v, attendu les deux plus récentes", db)
	}
	if _, err := os.Stat(autre); err != nil {
		t.Error("un fichier étranger a été supprimé")
	}
}

func TestPurgeZeroConserveTout(t *testing.T) {
	dossier := t.TempDir()
	for _, n := range []string{"vlpm-2026-09-01-030000.db", "vlpm-2026-09-02-030000.db"} {
		os.WriteFile(filepath.Join(dossier, n), []byte("x"), 0o600)
	}
	supprimes, err := PurgerSauvegardes(dossier, 0)
	if err != nil || len(supprimes) != 0 {
		t.Errorf("garder=0 doit tout conserver, supprimés = %v (err %v)", supprimes, err)
	}
}

// Deux sauvegardes rapprochées ne doivent pas entrer en collision : sinon la
// seconde échoue et la rotation qui la suit n'est jamais exécutée.
func TestDeuxSauvegardesRapprochees(t *testing.T) {
	st := baseTest(t)
	dossier := t.TempDir()

	for i := 0; i < 2; i++ {
		nom := NomSauvegarde(time.Now())
		if _, err := st.Sauvegarder(filepath.Join(dossier, nom)); err != nil {
			t.Fatalf("sauvegarde %d : %v", i+1, err)
		}
		time.Sleep(1100 * time.Millisecond) // les noms sont datés à la seconde
	}

	entrees, _ := os.ReadDir(dossier)
	if len(entrees) != 2 {
		t.Errorf("%d sauvegardes écrites, attendu 2", len(entrees))
	}
}
