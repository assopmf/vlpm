package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func incidentTest(t *testing.T, st *Store) int64 {
	t.Helper()
	i := &Incident{VehicleID: 1, UserID: 1, Type: "dommage", Gravite: "majeur",
		Description: "Pare-chocs enfoncé"}
	if err := st.CreateIncident(i); err != nil {
		t.Fatal(err)
	}
	return i.ID
}

// photoSurDisque crée un fichier au format que produit le serveur.
func photoSurDisque(t *testing.T, dossier string) string {
	t.Helper()
	nom, err := NomFichierPhoto(".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossier, nom), []byte("contenu"), 0o600); err != nil {
		t.Fatal(err)
	}
	return nom
}

func TestPhotosJointesALIncident(t *testing.T) {
	st := baseTest(t)
	id := incidentTest(t, st)
	dossier := t.TempDir()

	for i := 0; i < 2; i++ {
		p := &Photo{IncidentID: id, Fichier: photoSurDisque(t, dossier),
			TypeMIME: "image/jpeg", Octets: 7, Largeur: 800, Hauteur: 600}
		if err := st.CreatePhoto(p, 1); err != nil {
			t.Fatal(err)
		}
	}

	// Les photos doivent suivre l'incident dans les listes, sans appel séparé.
	liste, err := st.ListIncidents(0, "tous")
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || len(liste[0].Photos) != 2 {
		t.Fatalf("incident = %+v, attendu 2 photos jointes", liste)
	}
}

// Un incident sans photo doit exposer une liste vide, jamais nul : côté
// interface, la différence se paie en cas particuliers.
func TestIncidentSansPhotoRenvoieListeVide(t *testing.T) {
	st := baseTest(t)
	incidentTest(t, st)
	liste, _ := st.ListIncidents(0, "tous")
	if liste[0].Photos == nil {
		t.Error("Photos vaut nil, attendu une liste vide")
	}
}

// Le filet de sécurité : supprimer un incident doit finir par emporter ses
// photos du disque, quel que soit le chemin de suppression emprunté.
func TestBalayagePhotosOrphelines(t *testing.T) {
	st := baseTest(t)
	id := incidentTest(t, st)
	dossier := t.TempDir()

	garde := photoSurDisque(t, dossier)
	perdue := photoSurDisque(t, dossier)
	if err := st.CreatePhoto(&Photo{IncidentID: id, Fichier: garde,
		TypeMIME: "image/jpeg", Octets: 7}, 1); err != nil {
		t.Fatal(err)
	}
	// `perdue` existe sur le disque sans ligne correspondante.

	// Un fichier étranger au dossier ne doit pas être touché.
	etranger := filepath.Join(dossier, "notes.txt")
	os.WriteFile(etranger, []byte("x"), 0o600)

	n, err := BalayerPhotosOrphelines(st, dossier)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d fichier(s) supprimé(s), attendu 1", n)
	}
	if _, err := os.Stat(filepath.Join(dossier, garde)); err != nil {
		t.Error("la photo référencée a été supprimée")
	}
	if _, err := os.Stat(filepath.Join(dossier, perdue)); !os.IsNotExist(err) {
		t.Error("la photo orpheline est toujours là")
	}
	if _, err := os.Stat(etranger); err != nil {
		t.Error("un fichier étranger a été supprimé")
	}
}

// La purge RGPD supprime les incidents résolus anciens : leurs photos doivent
// disparaître avec eux, sinon l'engagement de suppression n'est pas tenu.
func TestPurgeRGPDEmporteLesPhotos(t *testing.T) {
	st := baseTest(t)
	dossier := t.TempDir()
	vieux := time.Now().UTC().AddDate(0, -30, 0).Format(time.RFC3339)

	res, err := st.DB.Exec(`INSERT INTO incidents
		(vehicle_id, user_id, type, gravite, description, statut, created_at, resolu_at)
		VALUES (1, 1, 'dommage', 'majeur', 'ancien', 'resolu', ?, ?)`, vieux, vieux)
	if err != nil {
		t.Fatal(err)
	}
	incidentID, _ := res.LastInsertId()

	fichier := photoSurDisque(t, dossier)
	if err := st.CreatePhoto(&Photo{IncidentID: incidentID, Fichier: fichier,
		TypeMIME: "image/jpeg", Octets: 7}, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Purger(Conservation{ActiviteMois: 12}); err != nil {
		t.Fatal(err)
	}
	// La ligne photo part en cascade avec l'incident.
	var reste int
	st.DB.QueryRow(`SELECT COUNT(*) FROM photos`).Scan(&reste)
	if reste != 0 {
		t.Errorf("%d ligne(s) photo restante(s) après purge de l'incident", reste)
	}

	n, err := BalayerPhotosOrphelines(st, dossier)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d fichier(s) balayé(s), attendu 1", n)
	}
	if _, err := os.Stat(filepath.Join(dossier, fichier)); !os.IsNotExist(err) {
		t.Error("la photo d'un incident purgé est restée sur le disque")
	}
}

// Un nom de fichier qui ne vient pas de notre générateur doit être refusé :
// c'est la protection contre une traversée de répertoire.
func TestCheminPhotoRefuseLesNomsForges(t *testing.T) {
	mauvais := []string{
		"../../etc/passwd", "../secret.jpg", "photo.jpg",
		"/etc/passwd", "", "0123456789abcdef0123456789abcdef.exe",
		"court.jpg", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz.jpg",
	}
	for _, nom := range mauvais {
		if _, err := CheminPhoto("/data/photos", nom); err == nil {
			t.Errorf("nom accepté à tort : %q", nom)
		}
	}

	bon, err := NomFichierPhoto(".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CheminPhoto("/data/photos", bon); err != nil {
		t.Errorf("nom légitime refusé : %v", err)
	}
}
