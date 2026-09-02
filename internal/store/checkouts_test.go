package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func baseTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("ouverture de la base : %v", err)
	}
	t.Cleanup(func() { st.Close() })

	u := &User{Matricule: "1204", Nom: "Martin", Prenom: "Camille",
		PasswordHash: "x", Role: "agent", Actif: true}
	if err := st.CreateUser(u); err != nil {
		t.Fatalf("création de l'agent : %v", err)
	}
	v := &Vehicle{Code: "TV1", Marque: "Peugeot", Modele: "5008",
		Statut: "disponible", KM: 45280, QRToken: "jeton-tv1"}
	if err := st.CreateVehicle(v); err != nil {
		t.Fatalf("création du véhicule : %v", err)
	}
	return st
}

func TestPriseEtRestitution(t *testing.T) {
	st := baseTest(t)

	c, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if err != nil {
		t.Fatalf("prise en compte : %v", err)
	}
	if c.Statut != "en_cours" {
		t.Errorf("statut de la prise = %q, attendu en_cours", c.Statut)
	}
	v, _ := st.VehicleByID(1)
	if v.Statut != "en_service" {
		t.Errorf("statut du véhicule = %q, attendu en_service", v.Statut)
	}

	fin, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 45412})
	if err != nil {
		t.Fatalf("restitution : %v", err)
	}
	if fin.Distance == nil || *fin.Distance != 132 {
		t.Errorf("distance = %v, attendu 132", fin.Distance)
	}
	v, _ = st.VehicleByID(1)
	if v.Statut != "disponible" || v.KM != 45412 {
		t.Errorf("après restitution : statut=%q km=%d, attendu disponible/45412", v.Statut, v.KM)
	}
}

// Le prototype affichait « + 18 053 km parcourus » pour une sortie de 24
// minutes : un écart pareil doit être refusé sans confirmation explicite.
func TestRestitutionKilometrageAberrant(t *testing.T) {
	st := baseTest(t)
	c, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 63333}); !errors.Is(err, ErrKMIncoherent) {
		t.Fatalf("erreur = %v, attendu ErrKMIncoherent", err)
	}
	// La sortie doit rester ouverte après un refus.
	if _, err := st.CheckoutEnCours(1); err != nil {
		t.Errorf("la prise en compte aurait dû rester en cours : %v", err)
	}

	// Avec confirmation, la valeur passe : le compteur a pu réellement bouger.
	if _, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 63333, Force: true}); err != nil {
		t.Fatalf("restitution forcée : %v", err)
	}
}

func TestRestitutionCompteurQuiRecule(t *testing.T) {
	st := baseTest(t)
	c, _ := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})

	// Même forcée, une marche arrière du compteur reste refusée.
	if _, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 45000, Force: true}); !errors.Is(err, ErrKMIncoherent) {
		t.Fatalf("erreur = %v, attendu ErrKMIncoherent", err)
	}
}

func TestDoublePriseEnCompteImpossible(t *testing.T) {
	st := baseTest(t)
	if _, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280}); err != nil {
		t.Fatal(err)
	}
	_, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if !errors.Is(err, ErrDejaEnService) {
		t.Fatalf("erreur = %v, attendu ErrDejaEnService", err)
	}
}

func TestPriseEnCompteVehiculeEnMaintenance(t *testing.T) {
	st := baseTest(t)
	if err := st.SetStatut(1, "maintenance"); err != nil {
		t.Fatal(err)
	}
	_, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if !errors.Is(err, ErrVehiculeIndisponible) {
		t.Fatalf("erreur = %v, attendu ErrVehiculeIndisponible", err)
	}
}

func TestRestitutionImmobilisante(t *testing.T) {
	st := baseTest(t)
	c, _ := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if _, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 45300, Immobilise: true}); err != nil {
		t.Fatal(err)
	}
	v, _ := st.VehicleByID(1)
	if v.Statut != "maintenance" {
		t.Errorf("statut = %q, attendu maintenance", v.Statut)
	}
}

func TestFmtKM(t *testing.T) {
	// \u202f est l'espace fine insécable utilisée comme séparateur de milliers.
	cas := map[int64]string{
		0: "0", 132: "132", 1500: "1\u202f500",
		18053: "18\u202f053", 1234567: "1\u202f234\u202f567",
	}
	for entree, attendu := range cas {
		if got := FmtKM(entree); got != attendu {
			t.Errorf("FmtKM(%d) = %q, attendu %q", entree, got, attendu)
		}
	}
}
