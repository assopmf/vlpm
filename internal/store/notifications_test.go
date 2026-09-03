package store

import (
	"testing"
	"time"
)

func TestDoitEnvoyerSelonFrequence(t *testing.T) {
	st := baseTest(t)
	maintenant := time.Now()

	// Désactivé par défaut : rien ne part tant que personne ne l'a demandé.
	if st.DoitEnvoyer(maintenant) {
		t.Error("un envoi est prévu alors que la fréquence est désactivée")
	}

	// Fréquence réglée, mais aucun destinataire : rien ne doit partir.
	if err := st.DefinirNotifications(Notifications{
		Frequence: FrequenceHebdomadaire, Destinataires: []string{}}); err != nil {
		t.Fatal(err)
	}
	if st.DoitEnvoyer(maintenant) {
		t.Error("un envoi est prévu sans destinataire")
	}

	// Avec un destinataire et aucun envoi passé : le premier relevé est dû.
	if err := st.DefinirNotifications(Notifications{
		Frequence:     FrequenceHebdomadaire,
		Destinataires: []string{"chef@ville.fr"}}); err != nil {
		t.Fatal(err)
	}
	if !st.DoitEnvoyer(maintenant) {
		t.Error("le premier relevé aurait dû être dû")
	}

	// Juste après un envoi : plus rien avant l'échéance.
	if err := st.MarquerEnvoi(maintenant); err != nil {
		t.Fatal(err)
	}
	if st.DoitEnvoyer(maintenant.Add(2 * 24 * time.Hour)) {
		t.Error("relevé hebdomadaire renvoyé au bout de deux jours")
	}
	if !st.DoitEnvoyer(maintenant.Add(7 * 24 * time.Hour)) {
		t.Error("relevé hebdomadaire non renvoyé au bout d'une semaine")
	}
}

// Sans la marge d'une heure, un passage à 8 h 00 puis 7 h 59 le lendemain
// repousserait l'envoi d'un jour entier, et de proche en proche.
func TestMargeContreLaDeriveHoraire(t *testing.T) {
	st := baseTest(t)
	st.DefinirNotifications(Notifications{
		Frequence: FrequenceQuotidienne, Destinataires: []string{"chef@ville.fr"}})

	envoi := time.Now()
	st.MarquerEnvoi(envoi)
	// Le lendemain, une minute plus tôt que la veille.
	lendemain := envoi.Add(24*time.Hour - time.Minute)
	if !st.DoitEnvoyer(lendemain) {
		t.Error("l'envoi quotidien décale d'un jour à chaque passage légèrement plus tôt")
	}
}

func TestDestinatairesReunisAvecLesChefs(t *testing.T) {
	st := baseTest(t)
	// L'agent créé par baseTest n'est pas chef : son adresse ne doit pas être
	// retenue, il n'a pas à recevoir le relevé du parc.
	st.DB.Exec(`UPDATE users SET email = 'agent@ville.fr' WHERE id = 1`)
	chef := &User{Matricule: "0801", Nom: "Fontaine", Prenom: "Julien",
		Email: "chef@ville.fr", PasswordHash: "x", Role: "chef", Actif: true}
	if err := st.CreateUser(chef); err != nil {
		t.Fatal(err)
	}
	// Un chef désactivé ne doit plus rien recevoir.
	inactif := &User{Matricule: "0802", Nom: "Parti", Prenom: "Ancien",
		Email: "ancien@ville.fr", PasswordHash: "x", Role: "chef", Actif: false}
	st.CreateUser(inactif)

	st.DefinirNotifications(Notifications{
		Frequence: FrequenceHebdomadaire, Destinataires: []string{"dgs@ville.fr"}})

	adresses, err := st.DestinatairesEffectifs()
	if err != nil {
		t.Fatal(err)
	}
	trouve := map[string]bool{}
	for _, a := range adresses {
		trouve[a] = true
	}
	if !trouve["dgs@ville.fr"] || !trouve["chef@ville.fr"] {
		t.Errorf("destinataires = %v, attendu le DGS et le chef", adresses)
	}
	if trouve["agent@ville.fr"] {
		t.Error("un agent simple reçoit le relevé du parc")
	}
	if trouve["ancien@ville.fr"] {
		t.Error("un chef désactivé reçoit encore le relevé")
	}
}

func TestFrequenceInconnueRefusee(t *testing.T) {
	st := baseTest(t)
	if err := st.DefinirNotifications(Notifications{Frequence: "mensuelle"}); err == nil {
		t.Error("une fréquence inconnue devrait être refusée")
	}
}
