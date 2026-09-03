// Package config rassemble les réglages de démarrage (drapeaux + variables
// d'environnement). Tout a une valeur par défaut utilisable telle quelle :
// lancer le binaire sans aucun argument doit fonctionner.
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr          string // adresse d'écoute, ex. ":8080"
	DataDir       string // dossier contenant la base et les sauvegardes
	BaseURL       string // URL publique, utilisée pour générer les QR codes
	Dev           bool   // sert le front depuis le disque au lieu de l'embarqué
	DerriereProxy bool   // fait confiance à X-Forwarded-For / X-Forwarded-Proto

	// SauvegardesGardees : nombre de sauvegardes quotidiennes conservées.
	// 0 les conserve toutes, une valeur négative désactive la sauvegarde
	// automatique.
	SauvegardesGardees int

	// Relais SMTP. Configuré à l'installation et non depuis l'interface : un
	// mot de passe saisi dans l'application serait stocké en clair dans la
	// base, alors que les mots de passe des agents n'y sont que hachés.
	SMTPHote         string
	SMTPPort         int
	SMTPUtilisateur  string
	SMTPMotDePasse   string
	SMTPExpediteur   string
	SMTPTLSImplicite bool
	NomService       string // affiché comme expéditeur et en tête des relevés
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "vlpm.db") }

// Charger lit les drapeaux puis les variables d'environnement VLPM_*.
// Les drapeaux explicitement fournis l'emportent sur l'environnement.
func Charger(args []string) (Config, error) {
	c := Config{
		Addr:    ":8080",
		DataDir: defautDataDir(),
	}

	fs := flag.NewFlagSet("vlpm", flag.ContinueOnError)
	fs.StringVar(&c.Addr, "addr", env("VLPM_ADDR", c.Addr), "adresse d'écoute (ex. :8080 ou 127.0.0.1:8080)")
	fs.StringVar(&c.DataDir, "data", env("VLPM_DATA", c.DataDir), "dossier des données (base SQLite, sauvegardes)")
	fs.StringVar(&c.BaseURL, "base-url", env("VLPM_BASE_URL", ""), "URL publique de l'application, pour les QR codes")
	fs.BoolVar(&c.Dev, "dev", envBool("VLPM_DEV", false), "mode développement : front rechargé depuis le disque")
	fs.BoolVar(&c.DerriereProxy, "derriere-proxy", envBool("VLPM_DERRIERE_PROXY", false),
		"l'application est derrière un reverse proxy (nginx, Caddy, Traefik)")
	fs.IntVar(&c.SauvegardesGardees, "sauvegardes", envInt("VLPM_SAUVEGARDES", 14),
		"sauvegardes quotidiennes conservées (0 = toutes, -1 = désactiver)")
	fs.StringVar(&c.SMTPHote, "smtp-hote", env("VLPM_SMTP_HOTE", ""),
		"serveur SMTP pour l'envoi des relevés d'échéance (vide = pas d'envoi)")
	fs.IntVar(&c.SMTPPort, "smtp-port", envInt("VLPM_SMTP_PORT", 587), "port SMTP")
	fs.StringVar(&c.SMTPUtilisateur, "smtp-utilisateur", env("VLPM_SMTP_UTILISATEUR", ""),
		"identifiant SMTP (vide pour un relais interne sans authentification)")
	fs.StringVar(&c.SMTPMotDePasse, "smtp-mot-de-passe", env("VLPM_SMTP_MOT_DE_PASSE", ""),
		"mot de passe SMTP — préférez la variable d'environnement")
	fs.StringVar(&c.SMTPExpediteur, "smtp-expediteur", env("VLPM_SMTP_EXPEDITEUR", ""),
		"adresse d'expédition des relevés")
	fs.BoolVar(&c.SMTPTLSImplicite, "smtp-tls-implicite", envBool("VLPM_SMTP_TLS_IMPLICITE", false),
		"connexion chiffrée d'emblée (port 465) au lieu de STARTTLS")
	fs.StringVar(&c.NomService, "nom-service", env("VLPM_NOM_SERVICE", ""),
		"nom du service, ex. « Police municipale de Ville-Exemple »")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "VLPM — gestion du parc automobile d'une police municipale\n\nUsage :\n  vlpm [options]\n\nOptions :\n")
		fs.PrintDefaults()
		fmt.Fprintf(fs.Output(), "\nChaque option a son équivalent en variable d'environnement (VLPM_ADDR, VLPM_DATA, ...).\n")
	}
	if err := fs.Parse(args); err != nil {
		return c, err
	}

	if strings.TrimSpace(c.DataDir) == "" {
		return c, fmt.Errorf("le dossier de données ne peut pas être vide")
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs
	if err := os.MkdirAll(c.DataDir, 0o750); err != nil {
		return c, fmt.Errorf("création du dossier de données %s : %w", c.DataDir, err)
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")

	// Un relais sans adresse d'expédition serait refusé par le serveur au
	// premier envoi, plusieurs jours après l'installation. Autant le dire tout
	// de suite.
	if c.SMTPHote != "" && c.SMTPExpediteur == "" {
		return c, fmt.Errorf("--smtp-expediteur est obligatoire dès qu'un serveur SMTP est configuré")
	}
	return c, nil
}

// defautDataDir place la base à côté du binaire quand c'est possible (clé USB,
// dossier applicatif), sinon dans le dossier courant.
func defautDataDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "data")
		if err := os.MkdirAll(dir, 0o750); err == nil {
			return dir
		}
	}
	return "data"
}

func env(cle, def string) string {
	if v, ok := os.LookupEnv(cle); ok && v != "" {
		return v
	}
	return def
}

func envInt(cle string, def int) int {
	if v, ok := os.LookupEnv(cle); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(cle string, def bool) bool {
	if v, ok := os.LookupEnv(cle); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
