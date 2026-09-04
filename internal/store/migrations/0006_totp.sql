-- Second facteur d'authentification, facultatif.
--
-- Le projet étant destiné à des collectivités dont les réseaux diffèrent —
-- instance exposée sur Internet pour l'une, cantonnée à un VLAN interne pour
-- l'autre — aucune politique ne peut être imposée depuis le code. Le mécanisme
-- est fourni, chaque commune décide s'il s'applique et à qui.
--
-- Désactivé par défaut : une instance qui exigerait un second facteur sans que
-- personne l'ait décidé serait un défaut, pas une précaution.

ALTER TABLE users ADD COLUMN totp_secret TEXT;
-- Distingue le secret enregistré du second facteur réellement en service :
-- entre la génération du QR et la saisie du premier code, le compte ne doit
-- pas se retrouver verrouillé.
ALTER TABLE users ADD COLUMN totp_actif INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_confirme_at TEXT;
-- Dernier pas de temps accepté : interdit de rejouer un code intercepté
-- pendant les trente secondes où il reste mathématiquement valable.
ALTER TABLE users ADD COLUMN totp_dernier_pas INTEGER NOT NULL DEFAULT 0;

-- Codes de secours, à imprimer et conserver hors ligne. Sans eux, un téléphone
-- perdu ou réinitialisé rendrait le compte inaccessible.
CREATE TABLE codes_secours (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Condensat SHA-256 : les codes sont tirés au hasard avec une entropie
    -- suffisante, un hachage lent n'apporterait rien et il faut pouvoir en
    -- comparer dix à chaque tentative.
    empreinte  TEXT NOT NULL,
    utilise_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX idx_codes_secours_user ON codes_secours(user_id) WHERE utilise_at IS NULL;
