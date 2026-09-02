package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrVehiculeIndisponible = errors.New("véhicule indisponible")
	ErrDejaEnService        = errors.New("véhicule déjà pris en compte")
	ErrKMIncoherent         = errors.New("kilométrage incohérent")
)

// KMDeltaMax borne le nombre de kilomètres acceptés sans confirmation sur une
// seule prise en compte. Au-delà, c'est presque toujours une faute de frappe :
// le proto affichait "+18 053 km parcourus" pour une sortie de 24 minutes.
const KMDeltaMax = 1500

const checkoutCols = `c.id, c.vehicle_id, c.user_id, c.statut, c.started_at, c.km_start,
	c.ended_at, c.km_end, c.motif, c.notes_depart, c.notes_retour, c.check_depart, c.check_retour, c.cloture_par`

func scanCheckout(row interface{ Scan(...any) error }, joint bool) (*Checkout, error) {
	var c Checkout
	var endedAt sql.NullString
	var kmEnd, cloture sql.NullInt64
	dest := []any{&c.ID, &c.VehicleID, &c.UserID, &c.Statut, &c.StartedAt, &c.KMStart,
		&endedAt, &kmEnd, &c.Motif, &c.NotesDepart, &c.NotesRetour, &c.CheckDepart, &c.CheckRetour, &cloture}
	if joint {
		dest = append(dest, &c.VehicleCode, &c.UserNom)
	}
	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.EndedAt = ns(endedAt)
	c.KMEnd = ni(kmEnd)
	c.ClotureID = ni(cloture)
	if c.KMEnd != nil {
		d := *c.KMEnd - c.KMStart
		c.Distance = &d
	}
	return &c, nil
}

const checkoutJoin = ` FROM checkouts c
	JOIN vehicles v ON v.id = c.vehicle_id
	JOIN users u ON u.id = c.user_id`

const checkoutJoinCols = checkoutCols + `, v.code, u.prenom || ' ' || u.nom`

func (s *Store) CheckoutByID(id int64) (*Checkout, error) {
	return scanCheckout(s.DB.QueryRow(`SELECT `+checkoutJoinCols+checkoutJoin+` WHERE c.id = ?`, id), true)
}

// CheckoutEnCours renvoie la prise en compte active d'un véhicule, ou ErrNotFound.
func (s *Store) CheckoutEnCours(vehicleID int64) (*Checkout, error) {
	return scanCheckout(s.DB.QueryRow(`SELECT `+checkoutJoinCols+checkoutJoin+
		` WHERE c.vehicle_id = ? AND c.statut = 'en_cours'`, vehicleID), true)
}

// CheckoutEnCoursPourAgent renvoie le véhicule que l'agent a actuellement en main.
func (s *Store) CheckoutEnCoursPourAgent(userID int64) (*Checkout, error) {
	return scanCheckout(s.DB.QueryRow(`SELECT `+checkoutJoinCols+checkoutJoin+
		` WHERE c.user_id = ? AND c.statut = 'en_cours' ORDER BY c.started_at DESC LIMIT 1`, userID), true)
}

func (s *Store) checkoutsEnCoursParVehicule() (map[int64]Checkout, error) {
	rows, err := s.DB.Query(`SELECT ` + checkoutJoinCols + checkoutJoin + ` WHERE c.statut = 'en_cours'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]Checkout{}
	for rows.Next() {
		c, err := scanCheckout(rows, true)
		if err != nil {
			return nil, err
		}
		m[c.VehicleID] = *c
	}
	return m, rows.Err()
}

type CheckoutFiltre struct {
	VehicleID int64
	UserID    int64
	Statut    string // "", "en_cours", "termine"
	Depuis    string // date RFC3339 incluse
	Jusqua    string
	Limit     int
	Offset    int
}

func (s *Store) ListCheckouts(f CheckoutFiltre) ([]Checkout, error) {
	q := `SELECT ` + checkoutJoinCols + checkoutJoin + ` WHERE 1=1`
	args := []any{}
	if f.VehicleID > 0 {
		q += ` AND c.vehicle_id = ?`
		args = append(args, f.VehicleID)
	}
	if f.UserID > 0 {
		q += ` AND c.user_id = ?`
		args = append(args, f.UserID)
	}
	if f.Statut != "" && f.Statut != "tous" {
		q += ` AND c.statut = ?`
		args = append(args, f.Statut)
	}
	if f.Depuis != "" {
		q += ` AND c.started_at >= ?`
		args = append(args, f.Depuis)
	}
	if f.Jusqua != "" {
		q += ` AND c.started_at <= ?`
		args = append(args, f.Jusqua)
	}
	q += ` ORDER BY c.started_at DESC LIMIT ? OFFSET ?`
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	args = append(args, f.Limit, f.Offset)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Checkout{}
	for rows.Next() {
		c, err := scanCheckout(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

type PriseEnCompte struct {
	VehicleID int64
	UserID    int64
	KMStart   int64
	Motif     string
	Notes     string
	Check     string // JSON de l'état des lieux
	Force     bool   // passe outre l'alerte de kilométrage incohérent
}

// PrendreEnCompte confie un véhicule à un agent. L'opération est atomique : le
// véhicule passe "en service" et son compteur est recalé sur le km saisi.
func (s *Store) PrendreEnCompte(p PriseEnCompte) (*Checkout, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var statut string
	var kmActuel int64
	err = tx.QueryRow(`SELECT statut, km FROM vehicles WHERE id = ? AND archive = 0`, p.VehicleID).
		Scan(&statut, &kmActuel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	switch statut {
	case "en_service":
		return nil, ErrDejaEnService
	case "maintenance", "hors_service":
		return nil, fmt.Errorf("%w : statut %q", ErrVehiculeIndisponible, statut)
	}

	if p.KMStart < kmActuel {
		return nil, fmt.Errorf("%w : %d km saisis, or le compteur est déjà à %d km",
			ErrKMIncoherent, p.KMStart, kmActuel)
	}
	if !p.Force && p.KMStart-kmActuel > KMDeltaMax {
		return nil, fmt.Errorf("%w : écart de %d km depuis la dernière restitution (max %d sans confirmation)",
			ErrKMIncoherent, p.KMStart-kmActuel, KMDeltaMax)
	}

	if p.Check == "" {
		p.Check = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO checkouts
		(vehicle_id, user_id, statut, started_at, km_start, motif, notes_depart, check_depart)
		VALUES (?,?,'en_cours',?,?,?,?,?)`,
		p.VehicleID, p.UserID, now, p.KMStart, p.Motif, p.Notes, p.Check)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	if _, err := tx.Exec(`UPDATE vehicles SET statut='en_service', km=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`, p.KMStart, p.VehicleID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.CheckoutByID(id)
}

type Restitution struct {
	CheckoutID  int64
	KMEnd       int64
	Notes       string
	Check       string
	Immobilise  bool  // le véhicule part en maintenance au lieu de redevenir disponible
	ClotureePar int64 // renseigné si un chef clôture à la place de l'agent
	Force       bool
}

// Restituer clôt une prise en compte et remet le véhicule dans le parc.
func (s *Store) Restituer(r Restitution) (*Checkout, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var vehicleID, kmStart int64
	var statut string
	err = tx.QueryRow(`SELECT vehicle_id, km_start, statut FROM checkouts WHERE id = ?`, r.CheckoutID).
		Scan(&vehicleID, &kmStart, &statut)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if statut != "en_cours" {
		return nil, errors.New("cette prise en compte est déjà clôturée")
	}
	if r.KMEnd < kmStart {
		return nil, fmt.Errorf("%w : %d km au retour, contre %d km au départ",
			ErrKMIncoherent, r.KMEnd, kmStart)
	}
	if !r.Force && r.KMEnd-kmStart > KMDeltaMax {
		return nil, fmt.Errorf("%w : %d km parcourus sur une seule sortie (max %d sans confirmation)",
			ErrKMIncoherent, r.KMEnd-kmStart, KMDeltaMax)
	}

	if r.Check == "" {
		r.Check = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var cloture any
	if r.ClotureePar > 0 {
		cloture = r.ClotureePar
	}
	if _, err := tx.Exec(`UPDATE checkouts SET statut='termine', ended_at=?, km_end=?,
		notes_retour=?, check_retour=?, cloture_par=? WHERE id=?`,
		now, r.KMEnd, r.Notes, r.Check, cloture, r.CheckoutID); err != nil {
		return nil, err
	}

	nouveauStatut := "disponible"
	if r.Immobilise {
		nouveauStatut = "maintenance"
	}
	if _, err := tx.Exec(`UPDATE vehicles SET statut=?, km=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id=?`,
		nouveauStatut, r.KMEnd, vehicleID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.CheckoutByID(r.CheckoutID)
}
