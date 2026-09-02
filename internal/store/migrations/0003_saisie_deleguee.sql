-- Saisie d'une prise en compte pour le compte d'un agent.
--
-- Un équipage qui part sans son téléphone doit pouvoir être enregistré depuis
-- le poste. Jusqu'ici, la sortie était attribuée d'office à l'utilisateur
-- connecté : le chef qui saisissait apparaissait comme détenteur du véhicule,
-- et la main courante était fausse.
--
-- saisi_par distingue les deux rôles : user_id reste l'agent qui détient
-- réellement le véhicule et en répond, saisi_par indique qui a effectué la
-- saisie. Le pendant existait déjà au retour avec cloture_par.

ALTER TABLE checkouts ADD COLUMN saisi_par INTEGER REFERENCES users(id);

CREATE INDEX idx_checkouts_saisi_par ON checkouts(saisi_par) WHERE saisi_par IS NOT NULL;

-- Un agent ne détient qu'un véhicule à la fois. La règle était appliquée par
-- l'API ; la déplacer ici la rend insensible aux appels concurrents, désormais
-- possibles puisqu'un chef peut ouvrir une sortie au nom d'un tiers.
CREATE UNIQUE INDEX idx_checkouts_un_seul_par_agent
    ON checkouts(user_id) WHERE statut = 'en_cours';
