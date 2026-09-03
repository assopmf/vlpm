-- Photos jointes aux constats d'incident.
--
-- Les fichiers vivent sur le disque, dans <data>/photos, et non dans la base.
-- Deux raisons : une sauvegarde quotidienne par VACUUM INTO recopierait sinon
-- l'intégralité des photos chaque jour, et quatorze sauvegardes de plusieurs
-- gigaoctets sur le disque d'un poste de police n'est pas tenable. Les photos
-- étant immuables une fois écrites, une synchronisation incrémentale du
-- dossier de données suffit à les protéger.
--
-- La colonne fichier porte un nom généré par le serveur, jamais celui fourni
-- par l'appareil : un nom de fichier venant du client est une entrée non fiable.

CREATE TABLE photos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id INTEGER NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    fichier     TEXT NOT NULL UNIQUE,   -- nom opaque sur le disque
    type_mime   TEXT NOT NULL,
    octets      INTEGER NOT NULL,
    largeur     INTEGER NOT NULL DEFAULT 0,
    hauteur     INTEGER NOT NULL DEFAULT 0,
    ajoutee_par INTEGER REFERENCES users(id),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX idx_photos_incident ON photos(incident_id);
