// Package web embarque l'interface dans le binaire.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// FS renvoie la racine des fichiers statiques (le dossier "static" aplati).
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err) // impossible : le dossier est vérifié à la compilation
	}
	return sub
}
