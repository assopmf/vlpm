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
	"github.com/assopmf/vlpm/internal/courriel"
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

	// Sans relais configuré, l'expéditeur reste nil : l'interface le signale
	// plutôt que de laisser croire qu'un relevé partira.
	var expediteur courriel.Expediteur
	if cfg.SMTPHote != "" {
		expediteur = courriel.Nouveau(courriel.Config{
			Hote: cfg.SMTPHote, Port: cfg.SMTPPort,
			Utilisateur: cfg.SMTPUtilisateur, MotDePasse: cfg.SMTPMotDePasse,
			Expediteur: cfg.SMTPExpediteur, NomService: cfg.NomService,
			TLSImplicite: cfg.SMTPTLSImplicite,
		})
		log.Info("envoi de courriels actif", "relais", cfg.SMTPHote, "expediteur", cfg.SMTPExpediteur)
	}

	srv := api.New(cfg, st, authSvc, log, expediteur)
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
	go menage(ctx, authSvc, st, log)
	go entretienQuotidien(ctx, st, cfg, expediteur, log)

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

func menage(ctx context.Context, a *auth.Service, st *store.Store, log *slog.Logger) {
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
			if n, err := st.PurgerTentatives(); err != nil {
				log.Warn("purge des tentatives de connexion", "erreur", err)
			} else if n > 0 {
				log.Info("tentatives de connexion périmées supprimées", "nombre", n)
			}
		}
	}
}

// purgerDonnees applique les durées de conservation fixées par
// l'administrateur. Sans durée configurée, rien n'est supprimé.
func purgerDonnees(st *store.Store, log *slog.Logger) {
	c := st.Conservation()
	if c.ActiviteMois == 0 && c.JournalMois == 0 {
		return
	}
	bilan, err := st.Purger(c)
	if err != nil {
		log.Error("purge des données anciennes", "erreur", err)
		return
	}
	if bilan.Total() > 0 {
		log.Info("données anciennes purgées",
			"sorties", bilan.Sorties, "incidents", bilan.Incidents,
			"journal", bilan.Journal,
			"conservation_activite_mois", c.ActiviteMois,
			"conservation_journal_mois", c.JournalMois)
	}
}

// balayerPhotos supprime du disque les fichiers dont plus aucune ligne ne
// parle. Sans ce balayage, une purge de conservation effacerait les incidents
// mais laisserait leurs photos : la commune s'est engagée à les supprimer, et
// ce sont elles qui portent le plus de données personnelles.
func balayerPhotos(cfg config.Config, st *store.Store, log *slog.Logger) {
	n, err := store.BalayerPhotosOrphelines(st, filepath.Join(cfg.DataDir, "photos"))
	if err != nil {
		log.Error("balayage des photos orphelines", "erreur", err)
		return
	}
	if n > 0 {
		log.Info("photos orphelines supprimées", "nombre", n)
	}
}

// entretienQuotidien enchaîne, une fois par jour, la sauvegarde de la base
// puis la purge des données arrivées au terme de leur conservation.
//
// L'ordre compte : une sauvegarde fraîche existe toujours au moment où des
// enregistrements sont supprimés définitivement. Les deux tâches restent
// indépendantes — désactiver les sauvegardes ne doit pas désactiver une purge
// exigée par le RGPD.
func entretienQuotidien(ctx context.Context, st *store.Store, cfg config.Config,
	exp courriel.Expediteur, log *slog.Logger) {
	if cfg.SauvegardesGardees < 0 {
		log.Info("sauvegarde automatique désactivée")
	}

	passe := func() {
		if cfg.SauvegardesGardees >= 0 {
			sauvegarder(st, cfg, log)
		}
		purgerDonnees(st, log)
		balayerPhotos(cfg, st, log)
		envoyerReleve(st, cfg, exp, log)
	}

	// Un premier passage peu après le démarrage : sur une machine éteinte
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
			passe()
		case <-t.C:
			passe()
		}
	}
}

// envoyerReleve expédie le relevé d'échéances si l'un est dû.
//
// Aucun message n'est envoyé quand il n'y a rien à signaler : un relevé vide
// reçu chaque semaine finit dans la corbeille, et emporte les autres avec lui.
func envoyerReleve(st *store.Store, cfg config.Config, exp courriel.Expediteur, log *slog.Logger) {
	if exp == nil || !st.DoitEnvoyer(time.Now()) {
		return
	}
	alertes, err := st.Alertes()
	if err != nil {
		log.Error("relevé d'échéances : lecture", "erreur", err)
		return
	}
	if len(alertes) == 0 {
		return
	}
	destinataires, err := st.DestinatairesEffectifs()
	if err != nil {
		log.Error("relevé d'échéances : destinataires", "erreur", err)
		return
	}
	if len(courriel.AdressesValides(destinataires)) == 0 {
		return
	}

	sujet, corps := store.ComposerReleve(cfg.NomService, alertes, cfg.BaseURL)
	if err := exp.Envoyer(courriel.Message{
		Destinataires: destinataires, Sujet: sujet, Corps: corps,
	}); err != nil {
		// L'échec n'est pas fatal : le relevé repartira au prochain passage,
		// puisque la date d'envoi n'est pas marquée.
		log.Error("relevé d'échéances : envoi", "erreur", err)
		return
	}
	if err := st.MarquerEnvoi(time.Now()); err != nil {
		log.Warn("relevé d'échéances : horodatage", "erreur", err)
	}
	log.Info("relevé d'échéances envoyé", "alertes", len(alertes),
		"destinataires", len(courriel.AdressesValides(destinataires)))
}

// sauvegarder écrit un instantané de la base et fait tourner les anciens.
//
// La copie vit sur le même disque que l'original : elle protège d'une
// corruption ou d'une fausse manœuvre, pas d'une panne matérielle. Une copie
// hors machine reste indispensable, et le README l'explique.
func sauvegarder(st *store.Store, cfg config.Config, log *slog.Logger) {
	dossier := filepath.Join(cfg.DataDir, "sauvegardes")
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
