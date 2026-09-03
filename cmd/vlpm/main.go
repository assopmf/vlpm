// Commande vlpm : serveur autonome de gestion du parc automobile d'une police
// municipale. Un seul binaire, une base SQLite, aucune dépendance externe.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/assopmf/vlpm/internal/api"
	"github.com/assopmf/vlpm/internal/auth"
	"github.com/assopmf/vlpm/internal/config"
	"github.com/assopmf/vlpm/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nErreur : %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Les sous-commandes utilitaires n'ont pas besoin du serveur.
	if traite, err := executerSousCommande(os.Args[1:]); traite {
		return err
	}

	cfg, err := config.Charger(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // --help : le message a déjà été affiché
		}
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()
	log.Info("base de données prête", "fichier", cfg.DBPath())

	authSvc := auth.New(st)
	if err := amorcerAdmin(st, log); err != nil {
		return err
	}

	srv := api.New(cfg, st, authSvc, log)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Ménage périodique : sessions expirées et compteurs de tentatives.
	ctx, arreterMenage := context.WithCancel(context.Background())
	defer arreterMenage()
	go menage(ctx, authSvc, srv, log)
	go sauvegardeAutomatique(ctx, st, cfg, log)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("écoute sur %s : %w (le port est peut-être déjà utilisé)", cfg.Addr, err)
	}

	afficherBanniere(cfg, ln.Addr().String())

	erreurs := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erreurs <- err
		}
	}()

	arret := make(chan os.Signal, 1)
	signal.Notify(arret, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-erreurs:
		return err
	case <-arret:
		log.Info("arrêt demandé, fermeture des connexions en cours")
	}

	ctxArret, annuler := context.WithTimeout(context.Background(), 15*time.Second)
	defer annuler()
	if err := httpSrv.Shutdown(ctxArret); err != nil {
		return fmt.Errorf("arrêt du serveur : %w", err)
	}
	log.Info("serveur arrêté proprement")
	return nil
}

func menage(ctx context.Context, a *auth.Service, srv *api.Server, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := a.PurgerSessions(); err != nil {
				log.Warn("purge des sessions", "erreur", err)
			} else if n > 0 {
				log.Info("sessions expirées supprimées", "nombre", n)
			}
			srv.PurgerLimiteur()
		}
	}
}

// sauvegardeAutomatique écrit un instantané quotidien de la base.
//
// Elle protège d'une corruption ou d'une fausse manœuvre, pas d'une panne du
// disque : la copie est au même endroit que l'original. Une copie hors machine
// reste indispensable, et le README le dit.
func sauvegardeAutomatique(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) {
	if cfg.SauvegardesGardees < 0 {
		log.Info("sauvegarde automatique désactivée")
		return
	}
	dossier := filepath.Join(cfg.DataDir, "sauvegardes")

	ecrire := func() {
		fichier := filepath.Join(dossier, store.NomSauvegarde(time.Now()))
		taille, err := st.Sauvegarder(fichier)
		if err != nil {
			log.Error("sauvegarde automatique", "erreur", err)
			return
		}
		log.Info("sauvegarde écrite", "fichier", fichier, "octets", taille)

		if supprimes, err := store.PurgerSauvegardes(dossier, cfg.SauvegardesGardees); err != nil {
			log.Warn("purge des anciennes sauvegardes", "erreur", err)
		} else if len(supprimes) > 0 {
			log.Info("anciennes sauvegardes supprimées", "nombre", len(supprimes))
		}
	}

	// Une première sauvegarde peu après le démarrage : sur une machine éteinte
	// chaque soir, un rythme strictement quotidien n'aboutirait jamais.
	premiere := time.NewTimer(5 * time.Minute)
	defer premiere.Stop()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-premiere.C:
			ecrire()
		case <-t.C:
			ecrire()
		}
	}
}

// amorcerAdmin crée le premier compte administrateur au tout premier démarrage
// et affiche ses identifiants dans la console. Aucun mot de passe par défaut
// n'est codé en dur : chaque installation a le sien.
func amorcerAdmin(st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	motDePasse, err := auth.MotDePasseAleatoire()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(motDePasse)
	if err != nil {
		return err
	}
	u := &store.User{
		Matricule: "admin", Nom: "Administrateur", Prenom: "Compte",
		PasswordHash: hash, Role: "admin", Actif: true, MustChange: true,
	}
	if err := st.CreateUser(u); err != nil {
		return fmt.Errorf("création du compte administrateur : %w", err)
	}

	fmt.Print("\n" + strings.Repeat("=", 64) + "\n")
	fmt.Println("  PREMIER DÉMARRAGE — compte administrateur créé")
	fmt.Println()
	fmt.Println("     Matricule    : admin")
	fmt.Println("     Mot de passe :", motDePasse)
	fmt.Println()
	fmt.Println("  Notez-le maintenant : il ne sera plus affiché.")
	fmt.Println("  Changez-le dès votre première connexion.")
	fmt.Print(strings.Repeat("=", 64) + "\n\n")
	log.Info("compte administrateur initial créé")
	return nil
}

func afficherBanniere(cfg config.Config, adresse string) {
	url := "http://" + adresse
	if strings.HasPrefix(adresse, "[::]") || strings.HasPrefix(adresse, "0.0.0.0") {
		_, port, _ := net.SplitHostPort(adresse)
		url = "http://localhost:" + port
	}
	fmt.Println()
	fmt.Println("  VLPM — gestion du parc automobile")
	fmt.Println("  ---------------------------------")
	fmt.Println("  Interface  :", url)
	fmt.Println("  API        :", url+"/api/v1")
	fmt.Println("  Données    :", cfg.DataDir)
	if cfg.BaseURL == "" {
		fmt.Println()
		fmt.Println("  Note : aucune URL publique configurée (--base-url).")
		fmt.Println("         Les QR codes fonctionneront depuis l'application,")
		fmt.Println("         mais pas en les scannant avec l'appareil photo du téléphone.")
	}
	fmt.Println()
	fmt.Println("  Ctrl+C pour arrêter.")
	fmt.Println()
}
