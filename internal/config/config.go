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
