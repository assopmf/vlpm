// Package totp implémente les codes à usage unique fondés sur le temps,
// tels que les génèrent Google Authenticator, FreeOTP ou Aegis.
//
// Spécifié par la RFC 6238, elle-même bâtie sur la RFC 4226. L'algorithme
// tient en quelques lignes avec la bibliothèque standard : y ajouter une
// dépendance externe pour cela n'aurait pas de sens dans un projet qui tient
// à rester compilable sans chaîne d'outils.
//
// SHA-1 est employé ici non par négligence mais par conformité : c'est ce que
// lisent les applications d'authentification du marché. Sa faiblesse connue
// porte sur la résistance aux collisions, sans effet sur HMAC.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Pas de temps : 30 secondes, valeur universellement attendue par les
	// applications d'authentification.
	Pas = 30 * time.Second
	// Chiffres composant le code.
	Chiffres = 6
	// Tolerance : nombre de pas acceptés de part et d'autre du pas courant.
	// Un pas absorbe une désynchronisation d'horloge d'une demi-minute et le
	// temps de saisie d'un code lu au dernier moment.
	Tolerance = 1
)

// NouveauSecret tire un secret de 160 bits, taille recommandée par la
// RFC 4226 pour HMAC-SHA1, et le rend en base32 sans remplissage — la seule
// forme que les applications d'authentification savent lire.
func NouveauSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Code calcule le code attendu pour un pas de temps donné.
func Code(secret string, pas int64) (string, error) {
	cle, err := decoder(secret)
	if err != nil {
		return "", err
	}

	var compteur [8]byte
	binary.BigEndian.PutUint64(compteur[:], uint64(pas))

	mac := hmac.New(sha1.New, cle)
	mac.Write(compteur[:])
	somme := mac.Sum(nil)

	// Troncature dynamique (RFC 4226 §5.3) : le dernier quartet désigne
	// l'endroit où prélever quatre octets.
	decalage := somme[len(somme)-1] & 0x0f
	tronque := binary.BigEndian.Uint32(somme[decalage:decalage+4]) & 0x7fffffff

	modulo := uint32(1)
	for i := 0; i < Chiffres; i++ {
		modulo *= 10
	}
	return fmt.Sprintf("%0*d", Chiffres, tronque%modulo), nil
}

// PasCourant renvoie le pas de temps correspondant à un instant.
func PasCourant(t time.Time) int64 { return t.Unix() / int64(Pas.Seconds()) }

// Verifier contrôle un code et renvoie le pas qui l'a validé.
//
// Le pas est renvoyé pour que l'appelant le mémorise : sans cela, un code
// intercepté resterait rejouable pendant toute sa durée de validité. La
// comparaison est faite en temps constant.
func Verifier(secret, saisi string, maintenant time.Time) (pas int64, ok bool) {
	saisi = strings.TrimSpace(strings.ReplaceAll(saisi, " ", ""))
	if len(saisi) != Chiffres {
		return 0, false
	}
	courant := PasCourant(maintenant)
	for d := -Tolerance; d <= Tolerance; d++ {
		attendu, err := Code(secret, courant+int64(d))
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(attendu), []byte(saisi)) == 1 {
			return courant + int64(d), true
		}
	}
	return 0, false
}

// URI compose le lien otpauth:// encodé dans le QR code d'inscription.
// L'émetteur et le libellé sont ce que l'application affichera : un agent doit
// reconnaître le compte parmi ceux qu'il a déjà.
func URI(secret, emetteur, compte string) string {
	if emetteur == "" {
		emetteur = "VLPM"
	}
	libelle := url.PathEscape(emetteur + ":" + compte)
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", emetteur)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprint(Chiffres))
	params.Set("period", fmt.Sprint(int(Pas.Seconds())))
	return "otpauth://totp/" + libelle + "?" + params.Encode()
}

// decoder accepte le secret tel qu'il est stocké comme tel qu'un utilisateur
// pourrait le recopier : minuscules, espaces et remplissage tolérés.
func decoder(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	s = strings.TrimRight(s, "=")
	cle, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("secret illisible : %w", err)
	}
	if len(cle) == 0 {
		return nil, fmt.Errorf("secret vide")
	}
	return cle, nil
}
