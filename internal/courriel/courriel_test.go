package courriel

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

// relaisFactice implémente juste assez du protocole SMTP pour recevoir un
// message et le restituer au test. Sans lui, on ne vérifierait que la
// composition du texte, jamais le dialogue réel avec un serveur.
type relaisFactice struct {
	adresse string
	mu      sync.Mutex
	recu    string
	de      string
	pour    []string
}

func demarrerRelais(t *testing.T) *relaisFactice {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &relaisFactice{adresse: ln.Addr().String()}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go r.servir(conn)
		}
	}()
	return r
}

func (r *relaisFactice) servir(conn net.Conn) {
	defer conn.Close()
	ecrire := func(s string) { conn.Write([]byte(s + "\r\n")) }
	ecrire("220 relais-test ESMTP")

	lecteur := bufio.NewReader(conn)
	for {
		ligne, err := lecteur.ReadString('\n')
		if err != nil {
			return
		}
		commande := strings.ToUpper(strings.TrimSpace(ligne))
		switch {
		case strings.HasPrefix(commande, "EHLO"), strings.HasPrefix(commande, "HELO"):
			// Pas de STARTTLS annoncé : le client doit s'en passer sans échouer.
			ecrire("250-relais-test")
			ecrire("250 SIZE 10240000")
		case strings.HasPrefix(commande, "MAIL FROM"):
			r.mu.Lock()
			r.de = extraireAdresse(ligne)
			r.mu.Unlock()
			ecrire("250 OK")
		case strings.HasPrefix(commande, "RCPT TO"):
			r.mu.Lock()
			r.pour = append(r.pour, extraireAdresse(ligne))
			r.mu.Unlock()
			ecrire("250 OK")
		case commande == "DATA":
			ecrire("354 Envoyez le message")
			var b strings.Builder
			for {
				l, err := lecteur.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				b.WriteString(l)
			}
			r.mu.Lock()
			r.recu = b.String()
			r.mu.Unlock()
			ecrire("250 OK")
		case commande == "QUIT":
			ecrire("221 Au revoir")
			return
		default:
			ecrire("250 OK")
		}
	}
}

func extraireAdresse(ligne string) string {
	debut := strings.Index(ligne, "<")
	fin := strings.Index(ligne, ">")
	if debut < 0 || fin < debut {
		return ""
	}
	return ligne[debut+1 : fin]
}

func (r *relaisFactice) message() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recu
}

func configTest(adresse string) Config {
	hote, port, _ := net.SplitHostPort(adresse)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return Config{
		Hote: hote, Port: p,
		Expediteur: "vlpm@ville-exemple.fr",
		NomService: "VLPM — Police municipale",
	}
}

func TestEnvoiComplet(t *testing.T) {
	relais := demarrerRelais(t)
	exp := Nouveau(configTest(relais.adresse))

	err := exp.Envoyer(Message{
		Destinataires: []string{"chef@ville-exemple.fr"},
		Sujet:         "3 échéances à traiter",
		Corps:         "Contrôle technique dépassé\nRévision proche",
	})
	if err != nil {
		t.Fatalf("envoi : %v", err)
	}

	recu := relais.message()
	// Le sujet doit être encodé : sans cela, « échéances » ressort illisible.
	if !strings.Contains(recu, "Subject: =?utf-8?") {
		t.Errorf("sujet non encodé RFC 2047 :\n%s", recu)
	}
	if !strings.Contains(recu, "Contrôle technique dépassé") {
		t.Error("corps absent du message")
	}
	if !strings.Contains(recu, "Auto-Submitted: auto-generated") {
		t.Error("en-tête de message automatique absent : risque de boucle de réponses d'absence")
	}
	if !strings.Contains(recu, `"VLPM — Police municipale" <vlpm@ville-exemple.fr>`) &&
		!strings.Contains(recu, "From: ") {
		t.Errorf("expéditeur mal formé :\n%s", recu)
	}
	if relais.de != "vlpm@ville-exemple.fr" {
		t.Errorf("MAIL FROM = %q", relais.de)
	}
	if len(relais.pour) != 1 || relais.pour[0] != "chef@ville-exemple.fr" {
		t.Errorf("RCPT TO = %v", relais.pour)
	}
}

// Une ligne réduite à un point termine le message au sens du protocole : elle
// doit être échappée, faute de quoi le courriel serait tronqué.
func TestPointSeulEchappe(t *testing.T) {
	relais := demarrerRelais(t)
	exp := Nouveau(configTest(relais.adresse))

	if err := exp.Envoyer(Message{
		Destinataires: []string{"chef@ville-exemple.fr"},
		Sujet:         "Test",
		Corps:         "Première ligne\n.\nDernière ligne",
	}); err != nil {
		t.Fatal(err)
	}
	recu := relais.message()
	if !strings.Contains(recu, "Dernière ligne") {
		t.Errorf("message tronqué par une ligne ne contenant qu'un point :\n%s", recu)
	}
}

func TestAdressesInvalidesEcartees(t *testing.T) {
	cas := []struct {
		entree  []string
		attendu int
	}{
		{[]string{"a@b.fr", "a@b.fr"}, 1}, // doublon
		{[]string{"a@b.fr", "A@B.FR"}, 1}, // doublon insensible à la casse
		{[]string{"pas-une-adresse", "a@b.fr"}, 1},
		{[]string{"", "  ", "a@b.fr"}, 1},
		{[]string{"a@b.fr", "c@d.fr"}, 2},
		{[]string{"Chef <chef@b.fr>"}, 1}, // forme avec nom
	}
	for _, k := range cas {
		if got := AdressesValides(k.entree); len(got) != k.attendu {
			t.Errorf("AdressesValides(%v) = %v, attendu %d adresse(s)", k.entree, got, k.attendu)
		}
	}
}

func TestSansDestinataireValide(t *testing.T) {
	relais := demarrerRelais(t)
	exp := Nouveau(configTest(relais.adresse))
	err := exp.Envoyer(Message{Destinataires: []string{"", "n'importe quoi"}, Sujet: "x"})
	if err == nil {
		t.Fatal("un envoi sans destinataire valide doit échouer")
	}
}

func TestNonConfigure(t *testing.T) {
	exp := Nouveau(Config{})
	if err := exp.Envoyer(Message{Destinataires: []string{"a@b.fr"}}); err != ErrNonConfigure {
		t.Fatalf("erreur = %v, attendu ErrNonConfigure", err)
	}
}
