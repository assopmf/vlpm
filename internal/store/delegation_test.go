package store

import (
	"errors"
	"testing"
)

// agentSupplementaire ajoute un second agent à la base de test.
func agentSupplementaire(t *testing.T, st *Store, matricule, nom string) *User {
	t.Helper()
	u := &User{Matricule: matricule, Nom: nom, Prenom: "Agent",
		PasswordHash: "x", Role: "agent", Actif: true}
	if err := st.CreateUser(u); err != nil {
		t.Fatalf("création de l'agent %s : %v", matricule, err)
	}
	return u
}

// Un chef saisit la sortie au nom d'un équipage parti sans son téléphone :
// le véhicule doit être attribué à l'agent, pas au chef qui saisit.
func TestPriseEnCompteAuNomDUnAgent(t *testing.T) {
	st := baseTest(t)
	chef := agentSupplementaire(t, st, "9000", "Chef")

	c, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, SaisiPar: chef.ID})
	if err != nil {
		t.Fatalf("prise en compte déléguée : %v", err)
	}
	if c.UserID != 1 {
		t.Errorf("détenteur = %d, attendu l'agent 1", c.UserID)
	}
	if c.SaisiParID == nil || *c.SaisiParID != chef.ID {
		t.Errorf("saisi_par = %v, attendu le chef %d", c.SaisiParID, chef.ID)
	}

	// L'agent voit bien le véhicule comme étant le sien.
	sien, err := st.CheckoutEnCoursPourAgent(1)
	if err != nil || sien.ID != c.ID {
		t.Errorf("la sortie devrait apparaître au nom de l'agent : %v", err)
	}
	// Et le chef n'est pas considéré comme détenteur.
	if _, err := st.CheckoutEnCoursPourAgent(chef.ID); !errors.Is(err, ErrNotFound) {
		t.Error("le chef ne doit pas apparaître comme détenteur du véhicule")
	}
}

// Une saisie faite par l'agent lui-même ne renseigne pas saisi_par : la colonne
// ne doit signaler qu'une délégation réelle.
func TestSaisieDirecteSansDelegation(t *testing.T) {
	st := baseTest(t)
	c, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, SaisiPar: 1})
	if err != nil {
		t.Fatal(err)
	}
	if c.SaisiParID != nil {
		t.Errorf("saisi_par = %v, attendu nil quand l'agent saisit lui-même", *c.SaisiParID)
	}
}

// Le chef peut équiper plusieurs équipages d'affilée : la limite du détenteur
// unique porte sur l'agent, pas sur celui qui saisit.
func TestChefEquipePlusieursEquipages(t *testing.T) {
	st := baseTest(t)
	chef := agentSupplementaire(t, st, "9000", "Chef")
	autre := agentSupplementaire(t, st, "1205", "Second")
	if err := st.CreateVehicle(&Vehicle{Code: "TV2", Statut: "disponible",
		KM: 38450, QRToken: "jeton-tv2"}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, SaisiPar: chef.ID}); err != nil {
		t.Fatalf("premier équipage : %v", err)
	}
	if _, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 2, UserID: autre.ID, KMStart: 38450, SaisiPar: chef.ID}); err != nil {
		t.Fatalf("second équipage : %v", err)
	}
}

// Un agent ne détient qu'un véhicule à la fois, même si c'est un chef qui
// enregistre la seconde sortie.
func TestAgentNeDetientQuUnVehicule(t *testing.T) {
	st := baseTest(t)
	chef := agentSupplementaire(t, st, "9000", "Chef")
	if err := st.CreateVehicle(&Vehicle{Code: "TV2", Statut: "disponible",
		KM: 38450, QRToken: "jeton-tv2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280}); err != nil {
		t.Fatal(err)
	}

	_, err := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 2, UserID: 1, KMStart: 38450, SaisiPar: chef.ID})
	if !errors.Is(err, ErrAgentDejaDetenteur) {
		t.Fatalf("erreur = %v, attendu ErrAgentDejaDetenteur", err)
	}
}

// L'agent au nom duquel la sortie a été ouverte peut la clôturer lui-même,
// sans que cela compte comme une clôture par un tiers.
func TestAgentRestitueUneSortieOuvertePourLui(t *testing.T) {
	st := baseTest(t)
	chef := agentSupplementaire(t, st, "9000", "Chef")
	c, _ := st.PrendreEnCompte(PriseEnCompte{
		VehicleID: 1, UserID: 1, KMStart: 45280, SaisiPar: chef.ID})

	fin, err := st.Restituer(Restitution{CheckoutID: c.ID, KMEnd: 45412})
	if err != nil {
		t.Fatal(err)
	}
	if fin.ClotureID != nil {
		t.Errorf("cloture_par = %v, attendu nil : l'agent a restitué lui-même", *fin.ClotureID)
	}
	if fin.SaisiParID == nil {
		t.Error("saisi_par doit rester renseigné après la restitution")
	}
}
