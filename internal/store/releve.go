package store

import (
	"fmt"
	"strings"
	"time"
)

// ComposerReleve rédige le relevé d'échéances envoyé aux responsables.
//
// Texte brut, volontairement court : un message qu'on ne lit pas jusqu'au bout
// ne sert à rien. Les échéances dépassées viennent en premier, ce sont les
// seules qui appellent une action immédiate.
func ComposerReleve(nomService string, alertes []Alerte, urlPublique string) (sujet, corps string) {
	depassees := []Alerte{}
	proches := []Alerte{}
	for _, a := range alertes {
		if a.Gravite == "depassee" {
			depassees = append(depassees, a)
		} else {
			proches = append(proches, a)
		}
	}

	switch {
	case len(depassees) > 0:
		sujet = fmt.Sprintf("%s à traiter, dont %s dépassée%s",
			pluriel(len(alertes), "échéance"), FmtKM(int64(len(depassees))),
			marque(len(depassees)))
	default:
		sujet = pluriel(len(alertes), "échéance") + " à prévoir"
	}

	var b strings.Builder
	if nomService != "" {
		fmt.Fprintf(&b, "%s\n\n", nomService)
	}
	b.WriteString("Relevé des échéances du parc automobile.\n")

	if len(depassees) > 0 {
		b.WriteString("\nDÉPASSÉES — action requise\n")
		for _, a := range depassees {
			fmt.Fprintf(&b, "  • %s : %s\n", etiquette(a), a.Message)
		}
	}
	if len(proches) > 0 {
		b.WriteString("\nÀ PRÉVOIR\n")
		for _, a := range proches {
			fmt.Fprintf(&b, "  • %s : %s\n", etiquette(a), a.Message)
		}
	}

	if urlPublique != "" {
		fmt.Fprintf(&b, "\nDétail et fiches véhicules : %s/alertes\n",
			strings.TrimRight(urlPublique, "/"))
	}
	b.WriteString("\n—\n")
	b.WriteString("Message automatique de VLPM. Pour ne plus le recevoir, modifiez\n")
	b.WriteString("la fréquence dans Réglages → Relevé d'échéances.\n")
	return sujet, b.String()
}

func etiquette(a Alerte) string {
	if a.Modele == "" {
		return a.Code
	}
	return a.Code + " (" + a.Modele + ")"
}

func pluriel(n int, mot string) string {
	return fmt.Sprintf("%s %s%s", FmtKM(int64(n)), mot, marque(n))
}

func marque(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// ProchainEnvoi indique quand le prochain relevé partira, pour l'afficher.
func (s *Store) ProchainEnvoi() string {
	n := s.Notifications()
	if n.Frequence == FrequenceDesactivee || n.DernierEnvoi == "" {
		return ""
	}
	dernier, err := time.Parse(time.RFC3339, n.DernierEnvoi)
	if err != nil {
		return ""
	}
	jours := 7
	if n.Frequence == FrequenceQuotidienne {
		jours = 1
	}
	return dernier.AddDate(0, 0, jours).Format(time.RFC3339)
}
