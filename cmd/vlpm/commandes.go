package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assopmf/vlpm/internal/auth"
	"github.com/assopmf/vlpm/internal/config"
	"github.com/assopmf/vlpm/internal/store"
)

// executerSousCommande traite les commandes utilitaires qui n'ont pas besoin
// de démarrer le serveur. Renvoie false si l'argument n'en est pas une, auquel
// cas le programme démarre normalement.
//
// Ces commandes agissent directement sur le fichier de base. Elles n'ont pas
// d'authentification : quiconque peut les lancer a déjà un accès en écriture à
// la base, et pourrait donc de toute façon la modifier avec n'importe quel
// outil SQLite. La protection réelle est celle du système de fichiers.
func executerSousCommande(args []string) (bool, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false, nil
	}
	switch args[0] {
	case "reinitialiser-admin":
		return true, cmdReinitialiserAdmin(args[1:])
	case "sauvegarder":
		return true, cmdSauvegarder(args[1:])
	case "aide", "help":
		afficherAide()
		return true, nil
	default:
		return true, fmt.Errorf("commande inconnue : %q\n\nLancez « vlpm aide » pour la liste des commandes.", args[0])
	}
}

func afficherAide() {
	fmt.Print(`VLPM — gestion du parc automobile d'une police municipale

Usage :
  vlpm [options]                    démarre le serveur
  vlpm sauvegarder [options]        écrit une sauvegarde de la base
  vlpm reinitialiser-admin [options]  redonne accès à un compte administrateur
  vlpm aide                         affiche ce message

Options du serveur :
  --addr            adresse d'écoute (défaut :8080)
  --data            dossier des données
  --base-url        URL publique, encodée dans les QR codes
  --derriere-proxy  l'application est derrière un reverse proxy
  --dev             interface rechargée depuis le disque

Chaque option a son équivalent en variable d'environnement (VLPM_ADDR, ...).
Ajoutez --help à une sous-commande pour connaître ses propres options.
`)
}

// ouvrirBase résout le dossier de données comme le fait le serveur, pour que
// les sous-commandes travaillent sur la même base sans avoir à la désigner.
func ouvrirBase(fs *flag.FlagSet, args []string, dossier *string) (*store.Store, error) {
	fs.StringVar(dossier, "data", env("VLPM_DATA", defautData()), "dossier des données")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(*dossier)
	if err != nil {
		return nil, err
	}
	*dossier = abs

	chemin := filepath.Join(abs, "vlpm.db")
	if _, err := os.Stat(chemin); err != nil {
		return nil, fmt.Errorf("aucune base à %s\n\nIndiquez le bon dossier avec --data, "+
			"ou démarrez d'abord le serveur pour la créer.", chemin)
	}
	return store.Open(chemin)
}

func env(cle, def string) string {
	if v, ok := os.LookupEnv(cle); ok && v != "" {
		return v
	}
	return def
}

func defautData() string {
	c, _ := config.Charger(nil)
	return c.DataDir
}

// cmdReinitialiserAdmin redonne accès à une instance dont le mot de passe
// administrateur a été perdu. Sans cette commande, il faudrait éditer la base
// à la main : un blocage total pour un service qui n'a pas de développeur.
func cmdReinitialiserAdmin(args []string) error {
	fs := flag.NewFlagSet("reinitialiser-admin", flag.ContinueOnError)
	matricule := fs.String("matricule", "admin", "matricule du compte à réinitialiser")
	var dossier string
	st, err := ouvrirBase(fs, args, &dossier)
	if err != nil {
		return err
	}
	defer st.Close()

	u, err := st.UserByMatricule(*matricule)
	if errors.Is(err, store.ErrNotFound) {
		// Le compte n'existe pas : on le crée plutôt que d'échouer, sinon une
		// instance dont l'unique admin a été supprimé resterait inaccessible.
		return creerAdmin(st, *matricule)
	}
	if err != nil {
		return err
	}

	motDePasse, err := auth.MotDePasseAleatoire()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(motDePasse)
	if err != nil {
		return err
	}
	if err := st.SetPassword(u.ID, hash, true); err != nil {
		return err
	}

	// Le compte doit aussi être actif et administrateur, sinon la
	// réinitialisation ne rendrait pas l'accès.
	remisEnEtat := []string{}
	if !u.Actif {
		u.Actif = true
		remisEnEtat = append(remisEnEtat, "réactivé")
	}
	if u.Role != "admin" {
		u.Role = "admin"
		remisEnEtat = append(remisEnEtat, "promu administrateur")
	}
	if len(remisEnEtat) > 0 {
		if err := st.UpdateUser(u); err != nil {
			return err
		}
	}

	// Les sessions ouvertes tombent : si le mot de passe a été perdu parce
	// qu'un appareil a été volé, il ne doit pas garder l'accès.
	if _, err := st.DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	st.Audit(0, "reinitialisation_admin_cli", "user", u.ID,
		"réinitialisation depuis la ligne de commande", "local")

	fmt.Print("\n" + strings.Repeat("=", 64) + "\n")
	fmt.Println("  ACCÈS ADMINISTRATEUR RÉTABLI")
	fmt.Println()
	fmt.Println("     Matricule    :", u.Matricule)
	fmt.Println("     Mot de passe :", motDePasse)
	if len(remisEnEtat) > 0 {
		fmt.Println("     Compte", strings.Join(remisEnEtat, " et "))
	}
	fmt.Println()
	fmt.Println("  Notez-le : il ne sera plus affiché.")
	fmt.Println("  Les sessions ouvertes de ce compte ont été fermées.")
	fmt.Print(strings.Repeat("=", 64) + "\n\n")
	return nil
}

func creerAdmin(st *store.Store, matricule string) error {
	motDePasse, err := auth.MotDePasseAleatoire()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(motDePasse)
	if err != nil {
		return err
	}
	u := &store.User{
		Matricule: matricule, Nom: "Administrateur", Prenom: "Compte",
		PasswordHash: hash, Role: "admin", Actif: true, MustChange: true,
	}
	if err := st.CreateUser(u); err != nil {
		return fmt.Errorf("création du compte %s : %w", matricule, err)
	}
	fmt.Print("\n" + strings.Repeat("=", 64) + "\n")
	fmt.Println("  COMPTE ADMINISTRATEUR CRÉÉ")
	fmt.Println()
	fmt.Println("     Matricule    :", matricule)
	fmt.Println("     Mot de passe :", motDePasse)
	fmt.Println()
	fmt.Println("  Notez-le : il ne sera plus affiché.")
	fmt.Print(strings.Repeat("=", 64) + "\n\n")
	return nil
}

// cmdSauvegarder produit un instantané de la base. Conçue pour être appelée
// par cron ou le Planificateur de tâches aussi bien qu'à la main.
func cmdSauvegarder(args []string) error {
	fs := flag.NewFlagSet("sauvegarder", flag.ContinueOnError)
	vers := fs.String("vers", "", "dossier de destination (défaut : <data>/sauvegardes)")
	garder := fs.Int("garder", 14, "nombre de sauvegardes à conserver (0 = toutes)")
	silencieux := fs.Bool("silencieux", false, "n'affiche rien en cas de succès (pour cron)")
	var dossier string
	st, err := ouvrirBase(fs, args, &dossier)
	if err != nil {
		return err
	}
	defer st.Close()

	destination := *vers
	if destination == "" {
		destination = filepath.Join(dossier, "sauvegardes")
	}
	fichier := filepath.Join(destination, store.NomSauvegarde(time.Now()))

	taille, err := st.Sauvegarder(fichier)
	if err != nil {
		return err
	}
	supprimes, err := store.PurgerSauvegardes(destination, *garder)
	if err != nil {
		// La sauvegarde est faite : un échec de purge ne doit pas la masquer.
		fmt.Fprintf(os.Stderr, "Avertissement : purge incomplète : %v\n", err)
	}

	if !*silencieux {
		fmt.Printf("Sauvegarde écrite : %s (%.1f Mo)\n", fichier, float64(taille)/(1<<20))
		if len(supprimes) > 0 {
			fmt.Printf("%d ancienne(s) sauvegarde(s) supprimée(s), %d conservée(s).\n",
				len(supprimes), *garder)
		}
		fmt.Println()
		fmt.Println("Cette sauvegarde est sur le même disque que la base : elle protège")
		fmt.Println("d'une corruption ou d'une fausse manœuvre, pas d'une panne de disque.")
		fmt.Println("Copiez-la régulièrement sur un autre support.")
	}
	return nil
}
