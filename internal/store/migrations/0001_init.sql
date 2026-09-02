-- VLPM — schéma initial (mono-commune)
-- Toutes les dates sont stockées en UTC au format RFC3339.

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    matricule     TEXT NOT NULL UNIQUE,
    nom           TEXT NOT NULL,
    prenom        TEXT NOT NULL,
    email         TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','chef','agent')),
    actif         INTEGER NOT NULL DEFAULT 1,
    must_change   INTEGER NOT NULL DEFAULT 0,
    derniere_connexion TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_users_actif ON users(actif);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE vehicles (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    code                TEXT NOT NULL UNIQUE,          -- TV1, TV2, ...
    marque              TEXT NOT NULL DEFAULT '',
    modele              TEXT NOT NULL DEFAULT '',
    immatriculation     TEXT NOT NULL DEFAULT '',
    categorie           TEXT NOT NULL DEFAULT 'patrouille',
    statut              TEXT NOT NULL DEFAULT 'disponible'
                        CHECK (statut IN ('disponible','en_service','maintenance','hors_service')),
    km                  INTEGER NOT NULL DEFAULT 0,
    date_mise_circulation TEXT,
    prochain_ct         TEXT,                          -- date du prochain contrôle technique
    prochaine_revision_km INTEGER,
    qr_token            TEXT NOT NULL UNIQUE,          -- identifiant opaque encodé dans le QR
    notes               TEXT NOT NULL DEFAULT '',
    archive             INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_vehicles_statut ON vehicles(statut);
CREATE INDEX idx_vehicles_archive ON vehicles(archive);

-- Prises en compte : un véhicule confié à un agent, puis restitué.
CREATE TABLE checkouts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    vehicle_id   INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    statut       TEXT NOT NULL DEFAULT 'en_cours' CHECK (statut IN ('en_cours','termine')),
    started_at   TEXT NOT NULL,
    km_start     INTEGER NOT NULL,
    ended_at     TEXT,
    km_end       INTEGER,
    motif        TEXT NOT NULL DEFAULT '',
    notes_depart TEXT NOT NULL DEFAULT '',
    notes_retour TEXT NOT NULL DEFAULT '',
    check_depart TEXT NOT NULL DEFAULT '{}',   -- JSON : état des lieux au départ
    check_retour TEXT NOT NULL DEFAULT '{}',   -- JSON : état des lieux au retour
    cloture_par  INTEGER REFERENCES users(id), -- si restitution forcée par un chef
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_checkouts_vehicle ON checkouts(vehicle_id, started_at DESC);
CREATE INDEX idx_checkouts_user ON checkouts(user_id, started_at DESC);
CREATE INDEX idx_checkouts_statut ON checkouts(statut);
-- Un seul véhicule en cours d'utilisation à la fois : garanti au niveau du moteur.
CREATE UNIQUE INDEX idx_checkouts_un_seul_en_cours
    ON checkouts(vehicle_id) WHERE statut = 'en_cours';

CREATE TABLE incidents (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    checkout_id INTEGER REFERENCES checkouts(id) ON DELETE SET NULL,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    type        TEXT NOT NULL DEFAULT 'autre'
                CHECK (type IN ('dommage','panne','proprete','carburant','equipement','autre')),
    gravite     TEXT NOT NULL DEFAULT 'mineur' CHECK (gravite IN ('mineur','majeur','immobilisant')),
    description TEXT NOT NULL DEFAULT '',
    statut      TEXT NOT NULL DEFAULT 'ouvert' CHECK (statut IN ('ouvert','en_cours','resolu')),
    resolu_par  INTEGER REFERENCES users(id),
    resolu_at   TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_incidents_vehicle ON incidents(vehicle_id, created_at DESC);
CREATE INDEX idx_incidents_statut ON incidents(statut);

CREATE TABLE maintenances (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    type        TEXT NOT NULL DEFAULT 'autre'
                CHECK (type IN ('revision','reparation','controle_technique','pneus','carburant','autre')),
    date        TEXT NOT NULL,
    km          INTEGER,
    cout_cents  INTEGER NOT NULL DEFAULT 0,
    prestataire TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_by  INTEGER REFERENCES users(id),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_maintenances_vehicle ON maintenances(vehicle_id, date DESC);

-- Journal d'audit : traçabilité exigée pour un usage en police municipale.
CREATE TABLE audit_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER REFERENCES users(id),
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL DEFAULT '',
    entity_id  INTEGER,
    details    TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
