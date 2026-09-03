// Package courriel compose et expédie les messages de service.
//
// La configuration SMTP passe par l'environnement ou les drapeaux, jamais par
// l'interface d'administration. Un mot de passe SMTP saisi dans l'application
// serait stocké en clair dans la base : les mots de passe des agents, eux,
// n'y sont que sous forme de condensat. Cette asymétrie serait un piège.
// L'installateur renseigne le relais une fois pour toutes ; les chefs règlent
// ensuite les destinataires et la fréquence, qui relèvent du service.
package courriel

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

var ErrNonConfigure = errors.New("aucun serveur d'envoi configuré")

type Config struct {
	Hote         string // serveur SMTP, vide = envoi désactivé
	Port         int
	Utilisateur  string
	MotDePasse   string
	Expediteur   string // adresse d'expédition
	NomService   string // nom affiché, ex. « VLPM — Police municipale de … »
	TLSImplicite bool   // port 465 : la connexion est chiffrée d'emblée
}

func (c Config) Active() bool { return strings.TrimSpace(c.Hote) != "" }

// Message est un courriel prêt à partir. Corps en texte brut uniquement : une
// notification de service n'a pas besoin de mise en forme, et le texte brut
// s'affiche correctement partout, y compris sur les webmails d'administration.
type Message struct {
	Destinataires []string
	Sujet         string
	Corps         string
}

type Expediteur interface {
	Envoyer(m Message) error
}

// SMTP expédie par un relais. Un relais interne sans authentification, cas
// fréquent en mairie, est pris en charge : les identifiants ne sont transmis
// que s'ils sont renseignés.
type SMTP struct{ cfg Config }

func Nouveau(cfg Config) *SMTP { return &SMTP{cfg: cfg} }

func (s *SMTP) Envoyer(m Message) error {
	if !s.cfg.Active() {
		return ErrNonConfigure
	}
	destinataires := adressesValides(m.Destinataires)
	if len(destinataires) == 0 {
		return errors.New("aucun destinataire valide")
	}

	adresse := net.JoinHostPort(s.cfg.Hote, fmt.Sprint(s.cfg.Port))
	client, err := s.connecter(adresse)
	if err != nil {
		return err
	}
	defer client.Close()

	if s.cfg.Utilisateur != "" {
		// PlainAuth de la bibliothèque standard refuse de transmettre des
		// identifiants sur une liaison en clair, sauf vers localhost. C'est le
		// comportement voulu : on ne le contourne pas.
		auth := smtp.PlainAuth("", s.cfg.Utilisateur, s.cfg.MotDePasse, s.cfg.Hote)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentification SMTP : %w", err)
		}
	}

	if err := client.Mail(s.cfg.Expediteur); err != nil {
		return fmt.Errorf("expéditeur refusé : %w", err)
	}
	for _, d := range destinataires {
		if err := client.Rcpt(d); err != nil {
			return fmt.Errorf("destinataire %s refusé : %w", d, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(s.composer(m, destinataires))); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *SMTP) connecter(adresse string) (*smtp.Client, error) {
	if s.cfg.TLSImplicite {
		conn, err := tls.Dial("tcp", adresse, &tls.Config{ServerName: s.cfg.Hote})
		if err != nil {
			return nil, fmt.Errorf("connexion TLS à %s : %w", adresse, err)
		}
		return smtp.NewClient(conn, s.cfg.Hote)
	}

	client, err := smtp.Dial(adresse)
	if err != nil {
		return nil, fmt.Errorf("connexion à %s : %w", adresse, err)
	}
	// STARTTLS dès que le relais l'annonce : sur un réseau communal, le trafic
	// passe par des équipements que nous ne maîtrisons pas.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Hote}); err != nil {
			client.Close()
			return nil, fmt.Errorf("passage en TLS : %w", err)
		}
	}
	return client, nil
}

// composer construit le message complet. Le sujet est encodé selon la
// RFC 2047, sans quoi les accents ressortent en caractères illisibles.
func (s *SMTP) composer(m Message, destinataires []string) string {
	nom := s.cfg.NomService
	if nom == "" {
		nom = "VLPM"
	}
	expediteur := (&mail.Address{Name: nom, Address: s.cfg.Expediteur}).String()

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", expediteur)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(destinataires, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Sujet))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	// Signale un message automatique : évite les réponses d'absence en boucle.
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("X-Auto-Response-Suppress: All\r\n")
	b.WriteString("\r\n")
	// Les points en début de ligne sont doublés : un point seul sur une ligne
	// termine le message au sens du protocole SMTP.
	b.WriteString(strings.ReplaceAll(normaliserFinsDeLigne(m.Corps), "\r\n.", "\r\n.."))
	return b.String()
}

func normaliserFinsDeLigne(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// adressesValides écarte les entrées vides ou mal formées, et supprime les
// doublons : un chef qui figure aussi dans la liste libre ne doit pas recevoir
// le message deux fois.
func adressesValides(liste []string) []string {
	vues := map[string]bool{}
	out := []string{}
	for _, a := range liste {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		adr, err := mail.ParseAddress(a)
		if err != nil {
			continue
		}
		cle := strings.ToLower(adr.Address)
		if vues[cle] {
			continue
		}
		vues[cle] = true
		out = append(out, adr.Address)
	}
	return out
}

// AdressesValides est exposée pour valider une saisie avant enregistrement.
func AdressesValides(liste []string) []string { return adressesValides(liste) }
