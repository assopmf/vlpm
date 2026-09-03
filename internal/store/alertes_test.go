package store

import (
	"testing"
	"time"
)

func dansNJours(n int) string {
	return time.Now().AddDate(0, 0, n).Format("2006-01-02")
}

func TestAlerteControleTechnique(t *testing.T) {
	st := baseTest(t)
	cas := []struct {
		nom      string
		echeance string
		attendue bool
		gravite  string
	}{
		{"dans 6 mois", dansNJours(180), false, ""},
		{"dans 31 jours", dansNJours(31), false, ""},
		{"dans 15 jours", dansNJours(15), true, "proche"},
		{"aujourd'hui", dansNJours(0), true, "depassee"},
		{"dépassé de 10 jours", dansNJours(-10), true, "depassee"},
	}
	for _, k := range cas {
		t.Run(k.nom, func(t *testing.T) {
			if _, err := st.DB.Exec(`UPDATE vehicles SET prochain_ct = ? WHERE id = 1`, k.echeance); err != nil {
				t.Fatal(err)
			}
			alertes, err := st.Alertes()
			if err != nil {
				t.Fatal(err)
			}
			if !k.attendue {
				if len(alertes) != 0 {
					t.Errorf("alerte inattendue : %+v", alertes)
				}
				return
			}
			if len(alertes) != 1 {
				t.Fatalf("%d alerte(s), attendu 1", len(alertes))
			}
			if alertes[0].Gravite != k.gravite {
				t.Errorf("gravité = %q, attendu %q (%s)", alertes[0].Gravite, k.gravite, alertes[0].Message)
			}
			if alertes[0].Type != "controle_technique" {
				t.Errorf("type = %q", alertes[0].Type)
			}
		})
	}
}

func TestAlerteRevision(t *testing.T) {
	st := baseTest(t)
	// Le véhicule de test est à 45 280 km.
	cas := []struct {
		nom      string
		seuil    int64
		attendue bool
		gravite  string
	}{
		{"seuil lointain", 60000, false, ""},
		{"seuil à 1500 km", 46780, false, ""},
		{"seuil à 500 km", 45780, true, "proche"},
		{"seuil atteint", 45280, true, "depassee"},
		{"seuil dépassé", 44000, true, "depassee"},
	}
	for _, k := range cas {
		t.Run(k.nom, func(t *testing.T) {
			if _, err := st.DB.Exec(`UPDATE vehicles SET prochaine_revision_km = ? WHERE id = 1`, k.seuil); err != nil {
				t.Fatal(err)
			}
			alertes, err := st.Alertes()
			if err != nil {
				t.Fatal(err)
			}
			if !k.attendue {
				if len(alertes) != 0 {
					t.Errorf("alerte inattendue : %+v", alertes)
				}
				return
			}
			if len(alertes) != 1 || alertes[0].Gravite != k.gravite {
				t.Fatalf("alertes = %+v, attendu une alerte %q", alertes, k.gravite)
			}
		})
	}
}

// Un véhicule archivé ne roule plus : ses échéances n'intéressent personne.
func TestAlerteIgnoreLesVehiculesArchives(t *testing.T) {
	st := baseTest(t)
	st.DB.Exec(`UPDATE vehicles SET prochain_ct = ?, archive = 1 WHERE id = 1`, dansNJours(-30))
	alertes, err := st.Alertes()
	if err != nil {
		t.Fatal(err)
	}
	if len(alertes) != 0 {
		t.Errorf("%d alerte(s) sur un véhicule archivé", len(alertes))
	}
}

// Sans échéance renseignée, aucune alerte ne doit être inventée.
func TestAucuneAlerteSansEcheance(t *testing.T) {
	st := baseTest(t)
	alertes, err := st.Alertes()
	if err != nil {
		t.Fatal(err)
	}
	if len(alertes) != 0 {
		t.Errorf("%d alerte(s) alors qu'aucune échéance n'est saisie", len(alertes))
	}
}

// Un véhicule peut cumuler les deux échéances.
func TestDeuxAlertesSurUnMemeVehicule(t *testing.T) {
	st := baseTest(t)
	st.DB.Exec(`UPDATE vehicles SET prochain_ct = ?, prochaine_revision_km = 45300 WHERE id = 1`,
		dansNJours(-5))
	alertes, err := st.Alertes()
	if err != nil {
		t.Fatal(err)
	}
	if len(alertes) != 2 {
		t.Fatalf("%d alerte(s), attendu 2", len(alertes))
	}
}

// Une date illisible ne doit ni planter ni produire d'alerte fantaisiste.
func TestDateIllisibleIgnoree(t *testing.T) {
	st := baseTest(t)
	st.DB.Exec(`UPDATE vehicles SET prochain_ct = 'bientot' WHERE id = 1`)
	alertes, err := st.Alertes()
	if err != nil {
		t.Fatalf("une date illisible ne doit pas provoquer d'erreur : %v", err)
	}
	if len(alertes) != 0 {
		t.Errorf("alerte produite depuis une date illisible : %+v", alertes)
	}
}
