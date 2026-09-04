package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// La RFC 6238 publie en annexe B des couples instant/code de référence. Les
// reproduire est la seule preuve que l'implémentation est conforme, et non
// simplement cohérente avec elle-même : un algorithme faux mais stable
// passerait n'importe quel test écrit à partir de sa propre sortie.
//
// Les vecteurs portent sur des codes à 8 chiffres ; Chiffres valant 6 en
// production, on compare les six derniers, la troncature étant identique.
func TestVecteursRFC6238(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))

	cas := []struct {
		instant int64
		code8   string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, k := range cas {
		got, err := Code(secret, k.instant/30)
		if err != nil {
			t.Fatal(err)
		}
		attendu := k.code8[len(k.code8)-Chiffres:]
		if got != attendu {
			t.Errorf("T=%d : code = %s, attendu %s (vecteur RFC %s)",
				k.instant, got, attendu, k.code8)
		}
	}
}

func TestVerificationDansLaFenetre(t *testing.T) {
	secret, err := NouveauSecret()
	if err != nil {
		t.Fatal(err)
	}
	maintenant := time.Now()
	code, _ := Code(secret, PasCourant(maintenant))

	if _, ok := Verifier(secret, code, maintenant); !ok {
		t.Error("le code courant est refusé")
	}
	// Un code lu au dernier moment doit encore passer au pas suivant.
	if _, ok := Verifier(secret, code, maintenant.Add(Pas)); !ok {
		t.Error("le code précédent est refusé alors que la tolérance est d'un pas")
	}
	// Au-delà de la tolérance, il doit être rejeté.
	if _, ok := Verifier(secret, code, maintenant.Add(3*Pas)); ok {
		t.Error("un code périmé de trois pas est accepté")
	}
}

// Le pas renvoyé permet d'interdire le rejeu : sans lui, un code intercepté
// resterait utilisable pendant toute sa fenêtre de validité.
func TestPasRenvoyePourEmpecherLeRejeu(t *testing.T) {
	secret, _ := NouveauSecret()
	maintenant := time.Now()
	code, _ := Code(secret, PasCourant(maintenant))

	pas, ok := Verifier(secret, code, maintenant)
	if !ok {
		t.Fatal("code refusé")
	}
	if pas != PasCourant(maintenant) {
		t.Errorf("pas renvoyé = %d, attendu %d", pas, PasCourant(maintenant))
	}
}

func TestSaisiesInvalides(t *testing.T) {
	secret, _ := NouveauSecret()
	maintenant := time.Now()
	for _, saisi := range []string{"", "12345", "1234567", "abcdef", "      "} {
		if _, ok := Verifier(secret, saisi, maintenant); ok {
			t.Errorf("saisie acceptée à tort : %q", saisi)
		}
	}
}

// Un agent recopiant le code avec l'espace que certaines applications
// affichent ne doit pas être refusé pour cette raison.
func TestEspacesTolerees(t *testing.T) {
	secret, _ := NouveauSecret()
	maintenant := time.Now()
	code, _ := Code(secret, PasCourant(maintenant))
	avecEspace := code[:3] + " " + code[3:]
	if _, ok := Verifier(secret, avecEspace, maintenant); !ok {
		t.Errorf("code refusé à cause d'un espace : %q", avecEspace)
	}
}

func TestSecretsDistincts(t *testing.T) {
	vus := map[string]bool{}
	for i := 0; i < 50; i++ {
		s, err := NouveauSecret()
		if err != nil {
			t.Fatal(err)
		}
		if vus[s] {
			t.Fatal("secret tiré deux fois")
		}
		vus[s] = true
	}
}

func TestURIExploitableParUneApplication(t *testing.T) {
	uri := URI("JBSWY3DPEHPK3PXP", "Police municipale de Ville-Exemple", "1204")
	for _, attendu := range []string{
		"otpauth://totp/", "secret=JBSWY3DPEHPK3PXP",
		"issuer=Police+municipale", "digits=6", "period=30", "algorithm=SHA1",
	} {
		if !strings.Contains(uri, attendu) {
			t.Errorf("URI ne contient pas %q :\n%s", attendu, uri)
		}
	}
}

// Un secret mal formé doit produire une erreur, jamais un code silencieusement
// faux qui laisserait croire à une configuration valide.
func TestSecretIllisible(t *testing.T) {
	if _, err := Code("pas-du-base32-!!", 0); err == nil {
		t.Error("un secret illisible devrait produire une erreur")
	}
	if _, err := Code("", 0); err == nil {
		t.Error("un secret vide devrait produire une erreur")
	}
}
