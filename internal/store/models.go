package store

import "database/sql"

type User struct {
	ID                int64  `json:"id"`
	Matricule         string `json:"matricule"`
	Nom               string `json:"nom"`
	Prenom            string `json:"prenom"`
	Email             string `json:"email"`
	PasswordHash      string `json:"-"`
	Role              string `json:"role"`
	Actif             bool   `json:"actif"`
	MustChange        bool   `json:"must_change_password"`
	DerniereConnexion string `json:"derniere_connexion,omitempty"`
	CreatedAt         string `json:"created_at"`
}

func (u User) NomComplet() string { return u.Prenom + " " + u.Nom }

// Peut renvoie true si le rôle de l'utilisateur atteint le niveau requis.
// Hiérarchie : agent < chef < admin.
func (u User) Peut(minRole string) bool {
	rang := map[string]int{"agent": 1, "chef": 2, "admin": 3}
	return rang[u.Role] >= rang[minRole]
}

type Vehicle struct {
	ID                  int64  `json:"id"`
	Code                string `json:"code"`
	Marque              string `json:"marque"`
	Modele              string `json:"modele"`
	Immatriculation     string `json:"immatriculation"`
	Categorie           string `json:"categorie"`
	Statut              string `json:"statut"`
	KM                  int64  `json:"km"`
	DateMiseCirculation string `json:"date_mise_circulation,omitempty"`
	ProchainCT          string `json:"prochain_ct,omitempty"`
	ProchaineRevisionKM *int64 `json:"prochaine_revision_km,omitempty"`
	QRToken             string `json:"qr_token,omitempty"`
	Notes               string `json:"notes"`
	Archive             bool   `json:"archive"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`

	// Champs calculés, remplis pour l'affichage.
	CheckoutEnCours  *Checkout `json:"checkout_en_cours,omitempty"`
	IncidentsOuverts int       `json:"incidents_ouverts"`
}

type Checkout struct {
	ID          int64  `json:"id"`
	VehicleID   int64  `json:"vehicle_id"`
	UserID      int64  `json:"user_id"`
	Statut      string `json:"statut"`
	StartedAt   string `json:"started_at"`
	KMStart     int64  `json:"km_start"`
	EndedAt     string `json:"ended_at,omitempty"`
	KMEnd       *int64 `json:"km_end,omitempty"`
	Motif       string `json:"motif"`
	NotesDepart string `json:"notes_depart"`
	NotesRetour string `json:"notes_retour"`
	CheckDepart string `json:"check_depart"`
	CheckRetour string `json:"check_retour"`
	ClotureID   *int64 `json:"cloture_par,omitempty"`

	// Traçabilité des opérations réalisées sans réseau. StartedAt et EndedAt
	// portent l'heure déclarée par l'appareil ; EnregistreAt et
	// RetourEnregistreAt l'heure de réception par le serveur. Un écart entre
	// les deux est normal après une synchronisation différée, et reste visible.
	DepartHorsLigne    bool   `json:"depart_hors_ligne"`
	RetourHorsLigne    bool   `json:"retour_hors_ligne"`
	EnregistreAt       string `json:"enregistre_at,omitempty"`
	RetourEnregistreAt string `json:"retour_enregistre_at,omitempty"`

	// Champs joints.
	VehicleCode string `json:"vehicle_code,omitempty"`
	UserNom     string `json:"user_nom,omitempty"`
	Distance    *int64 `json:"distance_km,omitempty"`
}

type Incident struct {
	ID          int64  `json:"id"`
	VehicleID   int64  `json:"vehicle_id"`
	CheckoutID  *int64 `json:"checkout_id,omitempty"`
	UserID      int64  `json:"user_id"`
	Type        string `json:"type"`
	Gravite     string `json:"gravite"`
	Description string `json:"description"`
	Statut      string `json:"statut"`
	ResoluAt    string `json:"resolu_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	CleClient   string `json:"-"` // idempotence des signalements différés

	VehicleCode string `json:"vehicle_code,omitempty"`
	UserNom     string `json:"user_nom,omitempty"`
}

type Maintenance struct {
	ID          int64  `json:"id"`
	VehicleID   int64  `json:"vehicle_id"`
	Type        string `json:"type"`
	Date        string `json:"date"`
	KM          *int64 `json:"km,omitempty"`
	CoutCents   int64  `json:"cout_cents"`
	Prestataire string `json:"prestataire"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`

	VehicleCode string `json:"vehicle_code,omitempty"`
}

type AuditEntry struct {
	ID        int64  `json:"id"`
	UserID    *int64 `json:"user_id,omitempty"`
	Action    string `json:"action"`
	Entity    string `json:"entity"`
	EntityID  *int64 `json:"entity_id,omitempty"`
	Details   string `json:"details"`
	IP        string `json:"ip"`
	CreatedAt string `json:"created_at"`
	UserNom   string `json:"user_nom,omitempty"`
}

// ns convertit un sql.NullString en string (chaîne vide si NULL).
func ns(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// ni convertit un sql.NullInt64 en pointeur (nil si NULL).
func ni(v sql.NullInt64) *int64 {
	if v.Valid {
		x := v.Int64
		return &x
	}
	return nil
}
