package store

import (
	"errors"
	"strings"
	"time"
)

var errFrequence = errors.New(
	"fréquence inconnue : valeurs acceptées desactivee, quotidienne, hebdomadaire")

// Réglages d'envoi des relevés d'échéance.
//
// La configuration du relais SMTP relève de l'installation ; ce qui suit
// relève du service, et se règle donc depuis l'administration : à qui, et à
// quelle fréquence.

const (
	CleNotifFrequence     = "notif_frequence"
	CleNotifDestinataires = "notif_destinataires"
	CleNotifDernierEnvoi  = "notif_dernier_envoi"
)

// Fréquences acceptées. « hebdomadaire » est le défaut : une échéance de
// contrôle technique se prépare sur des semaines, et un message quotidien
// répétant la même liste finit par ne plus être lu.
const (
	FrequenceDesactivee   = "desactivee"
	FrequenceQuotidienne  = "quotidienne"
	FrequenceHebdomadaire = "hebdomadaire"
)

type Notifications struct {
	Frequence     string   `json:"frequence"`
	Destinataires []string `json:"destinataires"`
	DernierEnvoi  string   `json:"dernier_envoi,omitempty"`
}

func (s *Store) Notifications() Notifications {
	n := Notifications{
		Frequence:     s.Setting(CleNotifFrequence, FrequenceDesactivee),
		DernierEnvoi:  s.Setting(CleNotifDernierEnvoi, ""),
		Destinataires: []string{},
	}
	if brut := strings.TrimSpace(s.Setting(CleNotifDestinataires, "")); brut != "" {
		for _, a := range strings.Split(brut, ",") {
			if a = strings.TrimSpace(a); a != "" {
				n.Destinataires = append(n.Destinataires, a)
			}
		}
	}
	return n
}

var frequencesValides = map[string]bool{
	FrequenceDesactivee: true, FrequenceQuotidienne: true, FrequenceHebdomadaire: true,
}

func (s *Store) DefinirNotifications(n Notifications) error {
	if !frequencesValides[n.Frequence] {
		return errFrequence
	}
	if err := s.SetSetting(CleNotifFrequence, n.Frequence); err != nil {
		return err
	}
	return s.SetSetting(CleNotifDestinataires, strings.Join(n.Destinataires, ","))
}

func (s *Store) MarquerEnvoi(t time.Time) error {
	return s.SetSetting(CleNotifDernierEnvoi, t.UTC().Format(time.RFC3339))
}

// DoitEnvoyer décide si un relevé est dû. Le calcul se fait sur la date du
// dernier envoi et non sur un minuteur : un serveur éteint la nuit, cas d'un
// poste allumé aux heures de service, ne doit pas sauter l'envoi.
func (s *Store) DoitEnvoyer(maintenant time.Time) bool {
	n := s.Notifications()
	if n.Frequence == FrequenceDesactivee || len(n.Destinataires) == 0 {
		return false
	}
	if n.DernierEnvoi == "" {
		return true
	}
	dernier, err := time.Parse(time.RFC3339, n.DernierEnvoi)
	if err != nil {
		return true // horodatage illisible : mieux vaut envoyer que se taire
	}
	intervalle := 7 * 24 * time.Hour
	if n.Frequence == FrequenceQuotidienne {
		intervalle = 24 * time.Hour
	}
	// Une heure de marge : sans elle, un passage à 8 h 00 puis 7 h 59 le
	// lendemain repousserait l'envoi d'une journée entière, de proche en proche.
	return maintenant.Sub(dernier) >= intervalle-time.Hour
}

// DestinatairesEffectifs réunit les adresses saisies et celles des chefs et
// administrateurs actifs : un responsable qui a renseigné son courriel n'a pas
// à être ajouté une seconde fois à la main.
func (s *Store) DestinatairesEffectifs() ([]string, error) {
	adresses := s.Notifications().Destinataires

	rows, err := s.DB.Query(`SELECT email FROM users
		WHERE actif = 1 AND role IN ('chef','admin') AND email != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		adresses = append(adresses, e)
	}
	return adresses, rows.Err()
}
