package store

import (
	"strings"
	"testing"
)

func TestReleveDistingueDepasseEtProche(t *testing.T) {
	alertes := []Alerte{
		{Code: "TV1", Modele: "Peugeot 5008", Type: "controle_technique",
			Gravite: "depassee", Message: "Contrôle technique dépassé depuis 8 jours"},
		{Code: "TV2", Modele: "Peugeot 308", Type: "controle_technique",
			Gravite: "proche", Message: "Contrôle technique dans 18 jours"},
	}
	sujet, corps := ComposerReleve("Police municipale de Ville-Exemple", alertes,
		"https://vlpm.ville-exemple.fr/")

	if !strings.Contains(sujet, "dépassée") {
		t.Errorf("sujet = %q : l'urgence doit apparaître dès l'objet", sujet)
	}
	// Les dépassées doivent précéder les autres.
	iDep := strings.Index(corps, "DÉPASSÉES")
	iProche := strings.Index(corps, "À PRÉVOIR")
	if iDep < 0 || iProche < 0 || iDep > iProche {
		t.Errorf("ordre des sections incorrect :\n%s", corps)
	}
	if !strings.Contains(corps, "TV1 (Peugeot 5008)") {
		t.Error("le véhicule concerné doit être identifiable")
	}
	if !strings.Contains(corps, "https://vlpm.ville-exemple.fr/alertes") {
		t.Errorf("lien absent ou mal construit :\n%s", corps)
	}
	// Le destinataire doit savoir comment se désabonner.
	if !strings.Contains(corps, "fréquence") {
		t.Error("aucune indication pour cesser de recevoir le message")
	}
}

func TestReleveSansDepassee(t *testing.T) {
	sujet, corps := ComposerReleve("", []Alerte{
		{Code: "TV2", Gravite: "proche", Message: "Révision dans 400 km"},
	}, "")
	if strings.Contains(sujet, "dépassée") {
		t.Errorf("sujet = %q, aucune échéance n'est dépassée", sujet)
	}
	if strings.Contains(corps, "DÉPASSÉES") {
		t.Error("section « dépassées » affichée à tort")
	}
	if strings.Contains(corps, "Détail et fiches") {
		t.Error("lien affiché alors qu'aucune URL publique n'est configurée")
	}
}

func TestRelevePluriel(t *testing.T) {
	sujet, _ := ComposerReleve("", []Alerte{{Code: "TV1", Gravite: "proche"}}, "")
	if strings.Contains(sujet, "échéances") {
		t.Errorf("sujet = %q, attendu le singulier", sujet)
	}
}
