package store

import (
	"database/sql"
	"errors"
	"time"
)

const incidentCols = `i.id, i.vehicle_id, i.checkout_id, i.user_id, i.type, i.gravite,
	i.description, i.statut, i.resolu_at, i.created_at, v.code, u.prenom || ' ' || u.nom`

const incidentJoin = ` FROM incidents i
	JOIN vehicles v ON v.id = i.vehicle_id
	JOIN users u ON u.id = i.user_id`

func scanIncident(row interface{ Scan(...any) error }) (*Incident, error) {
	var i Incident
	var checkoutID sql.NullInt64
	var resolu sql.NullString
	err := row.Scan(&i.ID, &i.VehicleID, &checkoutID, &i.UserID, &i.Type, &i.Gravite,
		&i.Description, &i.Statut, &resolu, &i.CreatedAt, &i.VehicleCode, &i.UserNom)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	i.CheckoutID = ni(checkoutID)
	i.ResoluAt = ns(resolu)
	return &i, nil
}

func (s *Store) ListIncidents(vehicleID int64, statut string) ([]Incident, error) {
	q := `SELECT ` + incidentCols + incidentJoin + ` WHERE 1=1`
	args := []any{}
	if vehicleID > 0 {
		q += ` AND i.vehicle_id = ?`
		args = append(args, vehicleID)
	}
	switch statut {
	case "ouverts":
		q += ` AND i.statut != 'resolu'`
	case "", "tous":
	default:
		q += ` AND i.statut = ?`
		args = append(args, statut)
	}
	q += ` ORDER BY i.created_at DESC LIMIT 300`

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *Store) incidentsOuvertsParVehicule() (map[int64]int, error) {
	rows, err := s.DB.Query(`SELECT vehicle_id, COUNT(*) FROM incidents
		WHERE statut != 'resolu' GROUP BY vehicle_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		m[id] = n
	}
	return m, rows.Err()
}

// CreateIncident enregistre un signalement. Une CleClient déjà vue signifie
// que le signalement avait été reçu : on renvoie ErrDejaEnregistre plutôt que
// d'en créer un doublon.
func (s *Store) CreateIncident(i *Incident) error {
	if i.CleClient != "" {
		var existant int64
		err := s.DB.QueryRow(`SELECT id FROM incidents WHERE cle_client = ?`, i.CleClient).Scan(&existant)
		if err == nil {
			i.ID = existant
			return ErrDejaEnregistre
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	var checkoutID any
	if i.CheckoutID != nil {
		checkoutID = *i.CheckoutID
	}
	res, err := s.DB.Exec(`INSERT INTO incidents
		(vehicle_id, checkout_id, user_id, type, gravite, description, statut, cle_client)
		VALUES (?,?,?,?,?,?,'ouvert',?)`,
		i.VehicleID, checkoutID, i.UserID, i.Type, i.Gravite, i.Description,
		nullIfEmpty(i.CleClient))
	if err != nil {
		return err
	}
	i.ID, _ = res.LastInsertId()
	i.Statut = "ouvert"
	return nil
}

func (s *Store) ResoudreIncident(id, userID int64) error {
	res, err := s.DB.Exec(`UPDATE incidents SET statut='resolu', resolu_par=?, resolu_at=?
		WHERE id=? AND statut != 'resolu'`, userID, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Maintenances ---

func (s *Store) ListMaintenances(vehicleID int64) ([]Maintenance, error) {
	q := `SELECT m.id, m.vehicle_id, m.type, m.date, m.km, m.cout_cents, m.prestataire,
		m.description, m.created_at, v.code
		FROM maintenances m JOIN vehicles v ON v.id = m.vehicle_id WHERE 1=1`
	args := []any{}
	if vehicleID > 0 {
		q += ` AND m.vehicle_id = ?`
		args = append(args, vehicleID)
	}
	q += ` ORDER BY m.date DESC, m.id DESC LIMIT 300`

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Maintenance{}
	for rows.Next() {
		var m Maintenance
		var km sql.NullInt64
		if err := rows.Scan(&m.ID, &m.VehicleID, &m.Type, &m.Date, &km, &m.CoutCents,
			&m.Prestataire, &m.Description, &m.CreatedAt, &m.VehicleCode); err != nil {
			return nil, err
		}
		m.KM = ni(km)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CreateMaintenance(m *Maintenance, userID int64) error {
	var km any
	if m.KM != nil {
		km = *m.KM
	}
	res, err := s.DB.Exec(`INSERT INTO maintenances
		(vehicle_id, type, date, km, cout_cents, prestataire, description, created_by)
		VALUES (?,?,?,?,?,?,?,?)`,
		m.VehicleID, m.Type, m.Date, km, m.CoutCents, m.Prestataire, m.Description, userID)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

// --- Audit ---

func (s *Store) Audit(userID int64, action, entity string, entityID int64, details, ip string) {
	var uid, eid any
	if userID > 0 {
		uid = userID
	}
	if entityID > 0 {
		eid = entityID
	}
	// L'audit ne doit jamais faire échouer l'action métier : on ignore l'erreur.
	_, _ = s.DB.Exec(`INSERT INTO audit_log (user_id, action, entity, entity_id, details, ip)
		VALUES (?,?,?,?,?,?)`, uid, action, entity, eid, details, ip)
}

func (s *Store) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT a.id, a.user_id, a.action, a.entity, a.entity_id, a.details,
		a.ip, a.created_at, COALESCE(u.prenom || ' ' || u.nom, '')
		FROM audit_log a LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var a AuditEntry
		var uid, eid sql.NullInt64
		if err := rows.Scan(&a.ID, &uid, &a.Action, &a.Entity, &eid, &a.Details,
			&a.IP, &a.CreatedAt, &a.UserNom); err != nil {
			return nil, err
		}
		a.UserID, a.EntityID = ni(uid), ni(eid)
		out = append(out, a)
	}
	return out, rows.Err()
}
