-- Durcissement de l'authentification.
--
-- 1. Les tentatives échouées étaient comptées en mémoire et par adresse IP
--    seulement. Un compte précis pouvait donc être attaqué depuis plusieurs
--    adresses sans jamais déclencher de blocage, et le compteur repartait à
--    zéro à chaque redémarrage du serveur — redémarrage désormais automatique.
--    Elles sont maintenant persistées et comptées sur les deux axes.
--
-- 2. Les sessions ne portaient pas l'adresse d'origine, ce qui empêchait un
--    agent de reconnaître ses propres appareils pour en révoquer un.

CREATE TABLE tentatives_connexion (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    matricule  TEXT NOT NULL,   -- tel que saisi, existant ou non
    ip         TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- Le matricule est enregistré qu'il corresponde ou non à un compte : sans
-- cela, la présence d'une ligne renseignerait sur l'existence du compte.
CREATE INDEX idx_tentatives_matricule ON tentatives_connexion(matricule, created_at);
CREATE INDEX idx_tentatives_ip ON tentatives_connexion(ip, created_at);
CREATE INDEX idx_tentatives_created ON tentatives_connexion(created_at);

ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT '';
