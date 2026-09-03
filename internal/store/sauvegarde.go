package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Sauvegarder écrit un instantané cohérent de la base dans le fichier indiqué.
//
// VACUUM INTO opère sur une base en cours d'utilisation : inutile d'arrêter le
// serveur, et le fichier produit est complet, sans journal WAL à côté. Une
// simple copie de vlpm.db, elle, laisserait des transactions dans le WAL et
// pourrait donner une sauvegarde tronquée.
func (s *Store) Sauvegarder(destination string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return 0, fmt.Errorf("création du dossier de sauvegarde : %w", err)
	}
	// VACUUM INTO refuse d'écraser un fichier existant.
	if _, err := os.Stat(destination); err == nil {
		return 0, fmt.Errorf("le fichier %s existe déjà", destination)
	}

	// Le chemin est interpolé faute de pouvoir le passer en paramètre lié :
	// VACUUM INTO n'accepte pas de placeholder. Les apostrophes sont donc
	// doublées, seul caractère significatif dans un littéral SQLite.
	chemin := strings.ReplaceAll(destination, "'", "''")
	if _, err := s.DB.Exec(`VACUUM INTO '` + chemin + `'`); err != nil {
		return 0, fmt.Errorf("sauvegarde vers %s : %w", destination, err)
	}

	info, err := os.Stat(destination)
	if err != nil {
		return 0, err
	}
	// La sauvegarde contient les mêmes données nominatives que la base.
	if err := os.Chmod(destination, 0o600); err != nil {
		return info.Size(), err
	}
	return info.Size(), nil
}

// NomSauvegarde compose un nom de fichier horodaté, triable par ordre
// alphabétique comme par ordre chronologique.
//
// Les secondes en font partie : sans elles, deux sauvegardes lancées dans la
// même minute entrent en collision, la seconde échoue, et la rotation qui suit
// n'est jamais exécutée. Le cas se produit dès qu'on relance une sauvegarde
// après un échec, ou qu'on en déclenche une à la main juste après l'automatique.
func NomSauvegarde(t time.Time) string {
	return "vlpm-" + t.Format("2006-01-02-150405") + ".db"
}

// PurgerSauvegardes ne conserve que les `garder` sauvegardes les plus
// récentes. Renvoie les fichiers supprimés.
func PurgerSauvegardes(dossier string, garder int) ([]string, error) {
	if garder <= 0 {
		return nil, nil // 0 signifie « tout conserver »
	}
	entrees, err := os.ReadDir(dossier)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	noms := []string{}
	for _, e := range entrees {
		// Ne touche qu'aux fichiers que nous avons produits : un dossier de
		// sauvegarde peut contenir autre chose.
		if !e.IsDir() && strings.HasPrefix(e.Name(), "vlpm-") && strings.HasSuffix(e.Name(), ".db") {
			noms = append(noms, e.Name())
		}
	}
	if len(noms) <= garder {
		return nil, nil
	}
	sort.Strings(noms) // l'horodatage rend l'ordre alphabétique chronologique

	supprimes := []string{}
	for _, nom := range noms[:len(noms)-garder] {
		chemin := filepath.Join(dossier, nom)
		if err := os.Remove(chemin); err != nil {
			return supprimes, fmt.Errorf("suppression de %s : %w", chemin, err)
		}
		supprimes = append(supprimes, nom)
	}
	return supprimes, nil
}
