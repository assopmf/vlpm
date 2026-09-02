package store

import (
	"errors"
	"testing"
	"time"
)

// Un réseau mobile instable peut faire aboutir une requête dont la réponse se
// perd. L'appareil réessaie : il ne doit pas en résulter deux sorties.
func TestRejeuPriseEnCompte(t *testing.T) {
	st := baseTest(t)

	c1, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, CleClient: "abc-123"})
	if err != nil {
		t.Fatalf("première prise en compte : %v", err)
	}

	c2, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, CleClient: "abc-123"})
	if !errors.Is(err, ErrDejaEnregistre) {
		t.Fatalf("erreur = %v, attendu ErrDejaEnregistre", err)
	}
	if c2 == nil || c2.ID != c1.ID {
		t.Fatalf("le rejeu aurait dû renvoyer la sortie %d, obtenu %v", c1.ID, c2)
	}

	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM checkouts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d sorties enregistrées, attendu 1", n)
	}
}

func TestRejeuRestitution(t *testing.T) {
	st := baseTest(t)
	c, _ := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})

	if _, err := st.Restituer(Restitution{
		CheckoutID: c.ID, KMEnd: 45412, CleClient: "retour-1"}); err != nil {
		t.Fatalf("première restitution : %v", err)
	}
	fin, err := st.Restituer(Restitution{
		CheckoutID: c.ID, KMEnd: 45412, CleClient: "retour-1"})
	if !errors.Is(err, ErrDejaEnregistre) {
		t.Fatalf("erreur = %v, attendu ErrDejaEnregistre", err)
	}
	if fin == nil || fin.KMEnd == nil || *fin.KMEnd != 45412 {
		t.Errorf("le rejeu aurait dû renvoyer la restitution existante, obtenu %v", fin)
	}
}

// L'heure retenue doit être celle de la sortie réelle, pas celle de la
// synchronisation : une main courante qui décale les sorties de quatre heures
// n'a aucune valeur.
func TestHorodatageDeclareConserve(t *testing.T) {
	st := baseTest(t)
	sortieReelle := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Second)

	c, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280,
		DebutDeclare: sortieReelle, HorsLigne: true})
	if err != nil {
		t.Fatal(err)
	}

	declare, err := time.Parse(time.RFC3339, c.StartedAt)
	if err != nil {
		t.Fatalf("started_at illisible : %v", err)
	}
	if !declare.Equal(sortieReelle) {
		t.Errorf("started_at = %v, attendu %v", declare, sortieReelle)
	}
	if !c.DepartHorsLigne {
		t.Error("depart_hors_ligne devrait être vrai")
	}

	// L'heure de réception par le serveur reste enregistrée à côté.
	recu, err := time.Parse(time.RFC3339, c.EnregistreAt)
	if err != nil {
		t.Fatalf("enregistre_at illisible : %v", err)
	}
	if recu.Sub(sortieReelle) < 3*time.Hour {
		t.Errorf("enregistre_at = %v : l'heure de réception devrait être bien postérieure", recu)
	}
}

func TestHorodatageAberrantRefuse(t *testing.T) {
	st := baseTest(t)

	cas := map[string]time.Time{
		"horloge en avance":   time.Now().UTC().Add(2 * time.Hour),
		"saisie trop vieille": time.Now().UTC().Add(-30 * 24 * time.Hour),
	}
	for nom, quand := range cas {
		t.Run(nom, func(t *testing.T) {
			_, err := st.PrendreEnCompte(PriseEnCompte{
				VehicleID: 1, UserID: 1, KMStart: 45280, DebutDeclare: quand})
			if !errors.Is(err, ErrHorodatageInvalide) {
				t.Fatalf("erreur = %v, attendu ErrHorodatageInvalide", err)
			}
		})
	}
}

// Une horloge déréglée ne doit pas produire une restitution antérieure à la
// sortie qu'elle clôt.
func TestRestitutionAnterieureAuDepartCorrigee(t *testing.T) {
	st := baseTest(t)
	c, _ := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})

	fin, err := st.Restituer(Restitution{
		CheckoutID: c.ID, KMEnd: 45412,
		RetourDeclare: time.Now().UTC().Add(-2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	debut, _ := time.Parse(time.RFC3339, fin.StartedAt)
	retour, _ := time.Parse(time.RFC3339, fin.EndedAt)
	if retour.Before(debut) {
		t.Errorf("retour %v antérieur au départ %v", retour, debut)
	}
}

// Une prise en compte sans clé cliente ne doit pas entrer en collision avec une
// autre : l'index unique accepte plusieurs NULL.
func TestPlusieursOperationsSansCle(t *testing.T) {
	st := baseTest(t)
	c, _ := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45280})
	if _, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 45300}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrendreEnCompte(PriseEnCompte{VehicleID: 1, UserID: 1, KMStart: 45300}); err != nil {
		t.Fatalf("deuxième sortie sans clé refusée : %v", err)
	}
}
