-- Prise en charge des opérations réalisées sans réseau.
--
-- Un agent qui prend un véhicule dans un parking souterrain enregistre son
-- action localement ; elle est transmise au serveur au retour du réseau.
-- Deux problèmes en découlent, traités ici.
--
-- 1. L'horodatage. La main courante doit indiquer l'heure réelle de la sortie,
--    pas celle de la synchronisation. On conserve donc l'heure déclarée par
--    l'appareil (started_at / ended_at) ET l'heure de réception par le serveur
--    (created_at / retour_enregistre_at). L'horloge d'un téléphone n'étant pas
--    une source de confiance, l'écart entre les deux reste visible au journal.
--
-- 2. Le rejeu. Un réseau instable peut faire aboutir une requête dont la
--    réponse se perd : l'appareil réessaie et créerait un doublon. Chaque
--    opération porte donc une clé générée par le client ; une clé déjà vue
--    renvoie l'enregistrement existant au lieu d'en créer un second.

ALTER TABLE checkouts ADD COLUMN cle_client TEXT;
ALTER TABLE checkouts ADD COLUMN cle_client_retour TEXT;
ALTER TABLE checkouts ADD COLUMN depart_hors_ligne INTEGER NOT NULL DEFAULT 0;
ALTER TABLE checkouts ADD COLUMN retour_hors_ligne INTEGER NOT NULL DEFAULT 0;
ALTER TABLE checkouts ADD COLUMN retour_enregistre_at TEXT;

-- Un index UNIQUE accepte plusieurs NULL sous SQLite : les enregistrements
-- créés depuis l'interface en ligne, sans clé, ne se gênent pas entre eux.
CREATE UNIQUE INDEX idx_checkouts_cle_client ON checkouts(cle_client)
    WHERE cle_client IS NOT NULL;
CREATE UNIQUE INDEX idx_checkouts_cle_client_retour ON checkouts(cle_client_retour)
    WHERE cle_client_retour IS NOT NULL;

-- Même mécanisme pour les incidents signalés hors ligne.
ALTER TABLE incidents ADD COLUMN cle_client TEXT;
CREATE UNIQUE INDEX idx_incidents_cle_client ON incidents(cle_client)
    WHERE cle_client IS NOT NULL;
