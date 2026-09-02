package store

import (
	"database/sql"
	"errors"
	"strings"
)

const vehicleCols = `v.id, v.code, v.marque, v.modele, v.immatriculation, v.categorie, v.statut, v.km,
	v.date_mise_circulation, v.prochain_ct, v.prochaine_revision_km, v.qr_token, v.notes, v.archive,
	v.created_at, v.updated_at`

func scanVehicle(row interface{ Scan(...any) error }) (*Vehicle, error) {
	var v Vehicle
	var dateMC, ct sql.NullString
	var revKM sql.NullInt64
	err := row.Scan(&v.ID, &v.Code, &v.Marque, &v.Modele, &v.Immatriculation, &v.Categorie,
		&v.Statut, &v.KM, &dateMC, &ct, &revKM, &v.QRToken, &v.Notes, &v.Archive,
		&v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.DateMiseCirculation = ns(dateMC)
	v.ProchainCT = ns(ct)
	v.ProchaineRevisionKM = ni(revKM)
	return &v, nil
}

func (s *Store) VehicleByID(id int64) (*Vehicle, error) {
	return scanVehicle(s.DB.QueryRow(`SELECT `+vehicleCols+` FROM vehicles v WHERE v.id = ?`, id))
}

func (s *Store) VehicleByCode(code string) (*Vehicle, error) {
	return scanVehicle(s.DB.QueryRow(`SELECT `+vehicleCols+` FROM vehicles v WHERE v.code = ? COLLATE NOCASE`,
		strings.TrimSpace(code)))
}

// VehicleByQR résout le jeton opaque encodé dans le QR collé dans le véhicule.
func (s *Store) VehicleByQR(token string) (*Vehicle, error) {
	return scanVehicle(s.DB.QueryRow(`SELECT `+vehicleCols+` FROM vehicles v WHERE v.qr_token = ?`,
		strings.TrimSpace(token)))
}

// ListVehicles renvoie le parc, avec pour chaque véhicule la prise en compte en
// cours (si elle existe) et le nombre d'incidents ouverts.
func (s *Store) ListVehicles(statut string, inclureArchives bool) ([]Vehicle, error) {
	q := `SELECT ` + vehicleCols + ` FROM vehicles v WHERE 1=1`
	args := []any{}
	if !inclureArchives {
		q += ` AND v.archive = 0`
	}
	if statut != "" && statut != "tous" {
		q += ` AND v.statut = ?`
		args = append(args, statut)
	}
	q += ` ORDER BY v.code COLLATE NOCASE`

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Vehicle{}
	ids := []int64{}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		v.QRToken = "" // jamais exposé dans une liste : il sert d'identifiant de scan
		out = append(out, *v)
		ids = append(ids, v.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	enCours, err := s.checkoutsEnCoursParVehicule()
	if err != nil {
		return nil, err
	}
	incidents, err := s.incidentsOuvertsParVehicule()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if c, ok := enCours[out[i].ID]; ok {
			cc := c
			out[i].CheckoutEnCours = &cc
		}
		out[i].IncidentsOuverts = incidents[out[i].ID]
	}
	return out, nil
}

func (s *Store) CreateVehicle(v *Vehicle) error {
	res, err := s.DB.Exec(`INSERT INTO vehicles
		(code, marque, modele, immatriculation, categorie, statut, km, date_mise_circulation,
		 prochain_ct, prochaine_revision_km, qr_token, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(v.Code), v.Marque, v.Modele, v.Immatriculation, v.Categorie, v.Statut, v.KM,
		nullIfEmpty(v.DateMiseCirculation), nullIfEmpty(v.ProchainCT), v.ProchaineRevisionKM,
		v.QRToken, v.Notes)
	if err != nil {
		return err
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateVehicle(v *Vehicle) error {
	_, err := s.DB.Exec(`UPDATE vehicles SET code=?, marque=?, modele=?, immatriculation=?, categorie=?,
		statut=?, km=?, date_mise_circulation=?, prochain_ct=?, prochaine_revision_km=?, notes=?, archive=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`,
		strings.TrimSpace(v.Code), v.Marque, v.Modele, v.Immatriculation, v.Categorie, v.Statut, v.KM,
		nullIfEmpty(v.DateMiseCirculation), nullIfEmpty(v.ProchainCT), v.ProchaineRevisionKM,
		v.Notes, v.Archive, v.ID)
	return err
}

// SetStatut ne change que le statut : utilisé par la mise en maintenance.
func (s *Store) SetStatut(vehicleID int64, statut string) error {
	_, err := s.DB.Exec(`UPDATE vehicles SET statut=?, updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id=?`, statut, vehicleID)
	return err
}

type Stats struct {
	Total       int `json:"total"`
	Disponibles int `json:"disponibles"`
	EnService   int `json:"en_service"`
	Maintenance int `json:"maintenance"`
	HorsService int `json:"hors_service"`
	Incidents   int `json:"incidents_ouverts"`
}

func (s *Store) Stats() (Stats, error) {
	var st Stats
	err := s.DB.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(statut='disponible'),0), COALESCE(SUM(statut='en_service'),0),
		COALESCE(SUM(statut='maintenance'),0), COALESCE(SUM(statut='hors_service'),0)
		FROM vehicles WHERE archive = 0`).
		Scan(&st.Total, &st.Disponibles, &st.EnService, &st.Maintenance, &st.HorsService)
	if err != nil {
		return st, err
	}
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM incidents WHERE statut != 'resolu'`).Scan(&st.Incidents)
	return st, err
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
