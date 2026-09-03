package store

import (
	"testing"
	"time"
)

// sortieAncienne fabrique une sortie terminée il y a n mois.
func sortieAncienne(t *testing.T, st *Store, vehicleID int64, moisEcoules int) int64 {
	t.Helper()
	debut := time.Now().UTC().AddDate(0, -moisEcoules, 0)
	fin := debut.Add(2 * time.Hour)
	res, err := st.DB.Exec(`INSERT INTO checkouts
		(vehicle_id, user_id, statut, started_at, km_start, ended_at, km_end)
		VALUES (?, 1, 'termine', ?, 1000, ?, 1100)`,
		vehicleID, debut.Format(time.RFC3339), fin.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func compter(t *testing.T, st *Store, table string) int {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPurgeRespecteLaDuree(t *testing.T) {
	st := baseTest(t)
	sortieAncienne(t, st, 1, 30) // 2 ans et demi
	sortieAncienne(t, st, 1, 18)
	sortieAncienne(t, st, 1, 6) // récente, à conserver
	sortieAncienne(t, st, 1, 1) // récente, à conserver

	c := Conservation{ActiviteMois: 12}

	// L'aperçu doit annoncer exactement ce que la purge fera.
	apercu, err := st.SimulerPurge(c)
	if err != nil {
		t.Fatal(err)
	}
	if apercu.Sorties != 2 {
		t.Fatalf("aperçu = %d sorties, attendu 2", apercu.Sorties)
	}
	if compter(t, st, "checkouts") != 4 {
		t.Error("l'aperçu ne doit rien supprimer")
	}

	bilan, err := st.Purger(c)
	if err != nil {
		t.Fatal(err)
	}
	if bilan.Sorties != apercu.Sorties {
		t.Errorf("purge = %d sorties, aperçu annonçait %d", bilan.Sorties, apercu.Sorties)
	}
	if reste := compter(t, st, "checkouts"); reste != 2 {
		t.Errorf("%d sorties restantes, attendu 2", reste)
	}
}

// Une sortie ouverte depuis très longtemps est une anomalie à traiter, pas une
// donnée à effacer : la purge ne doit jamais y toucher.
func TestPurgeEpargneLesSortiesEnCours(t *testing.T) {
	st := baseTest(t)
	vieux := time.Now().UTC().AddDate(0, -36, 0).Format(time.RFC3339)
	if _, err := st.DB.Exec(`INSERT INTO checkouts
		(vehicle_id, user_id, statut, started_at, km_start)
		VALUES (1, 1, 'en_cours', ?, 1000)`, vieux); err != nil {
		t.Fatal(err)
	}

	bilan, err := st.Purger(Conservation{ActiviteMois: 12})
	if err != nil {
		t.Fatal(err)
	}
	if bilan.Sorties != 0 {
		t.Errorf("%d sortie(s) supprimée(s), la sortie en cours devait être épargnée", bilan.Sorties)
	}
	if compter(t, st, "checkouts") != 1 {
		t.Error("la sortie en cours a été supprimée")
	}
}

// Un incident encore ouvert ne doit pas disparaître, même ancien.
func TestPurgeEpargneLesIncidentsOuverts(t *testing.T) {
	st := baseTest(t)
	vieux := time.Now().UTC().AddDate(0, -36, 0).Format(time.RFC3339)
	st.DB.Exec(`INSERT INTO incidents (vehicle_id, user_id, type, gravite, description, statut, created_at)
		VALUES (1, 1, 'panne', 'majeur', 'ouvert depuis longtemps', 'ouvert', ?)`, vieux)
	st.DB.Exec(`INSERT INTO incidents (vehicle_id, user_id, type, gravite, description, statut, created_at, resolu_at)
		VALUES (1, 1, 'proprete', 'mineur', 'résolu', 'resolu', ?, ?)`, vieux, vieux)

	bilan, err := st.Purger(Conservation{ActiviteMois: 12})
	if err != nil {
		t.Fatal(err)
	}
	if bilan.Incidents != 1 {
		t.Errorf("%d incident(s) supprimé(s), attendu 1 (le résolu seulement)", bilan.Incidents)
	}
	if compter(t, st, "incidents") != 1 {
		t.Error("l'incident ouvert aurait dû être conservé")
	}
}

// Par défaut, aucune donnée ne doit disparaître : une instance qui purgerait
// sans que personne l'ait décidé serait un défaut.
func TestAucunePurgeParDefaut(t *testing.T) {
	st := baseTest(t)
	sortieAncienne(t, st, 1, 60)

	c := st.Conservation()
	if c.ActiviteMois != 0 || c.JournalMois != 0 {
		t.Fatalf("conservation par défaut = %+v, attendu illimitée", c)
	}
	bilan, err := st.Purger(c)
	if err != nil {
		t.Fatal(err)
	}
	if bilan.Total() != 0 || compter(t, st, "checkouts") != 1 {
		t.Error("une sortie de 5 ans a été supprimée sans durée configurée")
	}
}

func TestDureesTropCourtesRefusees(t *testing.T) {
	st := baseTest(t)
	cas := []struct {
		nom string
		c   Conservation
	}{
		{"activité sous 12 mois", Conservation{ActiviteMois: 3}},
		{"journal sous 6 mois", Conservation{JournalMois: 1}},
		{"valeur négative", Conservation{ActiviteMois: -5}},
	}
	for _, k := range cas {
		t.Run(k.nom, func(t *testing.T) {
			if err := st.DefinirConservation(k.c); err == nil {
				t.Error("aurait dû être refusé")
			}
		})
	}
}

func TestConservationPersistee(t *testing.T) {
	st := baseTest(t)
	if err := st.DefinirConservation(Conservation{ActiviteMois: 24, JournalMois: 12}); err != nil {
		t.Fatal(err)
	}
	c := st.Conservation()
	if c.ActiviteMois != 24 || c.JournalMois != 12 {
		t.Errorf("relu = %+v, attendu 24/12", c)
	}
}

func TestJournalPurgeSeparement(t *testing.T) {
	st := baseTest(t)
	vieux := time.Now().UTC().AddDate(0, -24, 0).Format(time.RFC3339)
	st.DB.Exec(`INSERT INTO audit_log (action, entity, created_at) VALUES ('login', 'user', ?)`, vieux)
	st.DB.Exec(`INSERT INTO audit_log (action, entity) VALUES ('login', 'user')`)
	sortieAncienne(t, st, 1, 24)

	// Seul le journal est configuré : l'activité doit rester intacte.
	bilan, err := st.Purger(Conservation{JournalMois: 12})
	if err != nil {
		t.Fatal(err)
	}
	if bilan.Journal != 1 || bilan.Sorties != 0 {
		t.Errorf("bilan = %+v, attendu 1 entrée de journal et aucune sortie", bilan)
	}
	if compter(t, st, "checkouts") != 1 {
		t.Error("une sortie a été supprimée alors que seul le journal était configuré")
	}
}
