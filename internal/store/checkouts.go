package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrDejaEnregistre signale un rejeu : l'opération avait déjà été reçue.
	// L'enregistrement existant est renvoyé avec cette erreur, ce qui permet à
	// l'appelant de répondre un succès sans rien dupliquer.
	ErrDejaEnregistre = errors.New("opération déjà enregistrée")

	ErrVehiculeIndisponible = errors.New("véhicule indisponible")
	ErrAgentDejaDetenteur   = errors.New("cet agent détient déjà un véhicule")
	ErrDejaEnService        = errors.New("véhicule déjà pris en compte")
	ErrKMIncoherent         = errors.New("kilométrage incohérent")
	ErrHorodatageInvalide   = errors.New("horodatage invalide")
)

// KMDeltaMax borne le nombre de kilomètres acceptés sans confirmation sur une
// seule prise en compte. Au-delà, c'est presque toujours une faute de frappe :
// le proto affichait "+18 053 km parcourus" pour une sortie de 24 minutes.
const KMDeltaMax = 1500

// FmtKM met en forme un kilométrage à la française (« 18 053 »), pour que les
// messages du serveur se lisent comme ceux de l'interface.
func FmtKM(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	tete := len(s) % 3
	if tete > 0 {
		b.WriteString(s[:tete])
	}
	for i := tete; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString("\u202f") // espace fine insécable
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

const checkoutCols = `c.id, c.vehicle_id, c.user_id, c.statut, c.started_at, c.km_start,
	c.ended_at, c.km_end, c.motif, c.notes_depart, c.notes_retour, c.check_depart, c.check_retour,
	c.cloture_par, c.depart_hors_ligne, c.retour_hors_ligne, c.created_at, c.retour_enregistre_at,
	c.saisi_par`

func scanCheckout(row interface{ Scan(...any) error }, joint bool) (*Checkout, error) {
	var c Checkout
	var endedAt, retourEnregistre sql.NullString
	var kmEnd, cloture, saisiPar sql.NullInt64
	dest := []any{&c.ID, &c.VehicleID, &c.UserID, &c.Statut, &c.StartedAt, &c.KMStart,
		&endedAt, &kmEnd, &c.Motif, &c.NotesDepart, &c.NotesRetour, &c.CheckDepart, &c.CheckRetour,
		&cloture, &c.DepartHorsLigne, &c.RetourHorsLigne, &c.EnregistreAt, &retourEnregistre,
		&saisiPar}
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
	c.RetourEnregistreAt = ns(retourEnregistre)
	c.KMEnd = ni(kmEnd)
	c.ClotureID = ni(cloture)
	c.SaisiParID = ni(saisiPar)
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

	// Renseignés lorsque l'opération a été saisie sans réseau puis transmise
	// après coup. CleClient rend le rejeu inoffensif ; DebutDeclare porte
	// l'heure réelle de la sortie, telle que l'appareil l'a relevée.
	CleClient    string
	DebutDeclare time.Time
	HorsLigne    bool

	// SaisiPar est renseigné lorsqu'un chef enregistre la sortie au nom de
	// l'agent : UserID reste celui qui détient le véhicule et en répond.
	SaisiPar int64
}

// PrendreEnCompte confie un véhicule à un agent. L'opération est atomique : le
// véhicule passe "en service" et son compteur est recalé sur le km saisi.
func (s *Store) PrendreEnCompte(p PriseEnCompte) (*Checkout, error) {
	// Rejeu d'une opération déjà reçue : on renvoie l'enregistrement existant
	// plutôt que d'en créer un second. Le cas se produit dès qu'une réponse se
	// perd sur un réseau mobile instable.
	if p.CleClient != "" {
		if c, err := s.checkoutParCle("cle_client", p.CleClient); err == nil {
			return c, ErrDejaEnregistre
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

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

	// L'agent ne peut détenir qu'un véhicule à la fois. Le contrôle est dans la
	// transaction, et non côté API, car un chef peut désormais ouvrir une
	// sortie au nom d'un tiers : deux saisies simultanées pour le même agent
	// sont possibles.
	var dejaCode string
	err = tx.QueryRow(`SELECT v.code FROM checkouts c JOIN vehicles v ON v.id = c.vehicle_id
		WHERE c.user_id = ? AND c.statut = 'en_cours'`, p.UserID).Scan(&dejaCode)
	if err == nil {
		return nil, fmt.Errorf("%w : %s", ErrAgentDejaDetenteur, dejaCode)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if p.KMStart < kmActuel {
		return nil, fmt.Errorf("%w : %s km saisis, or le compteur est déjà à %s km",
			ErrKMIncoherent, FmtKM(p.KMStart), FmtKM(kmActuel))
	}
	if !p.Force && p.KMStart-kmActuel > KMDeltaMax {
		return nil, fmt.Errorf("%w : écart de %s km depuis la dernière restitution (maximum %s sans confirmation)",
			ErrKMIncoherent, FmtKM(p.KMStart-kmActuel), FmtKM(KMDeltaMax))
	}

	if p.Check == "" {
		p.Check = "{}"
	}
	debut, err := horodatageDeclare(p.DebutDeclare)
	if err != nil {
		return nil, err
	}
	var saisiPar any
	if p.SaisiPar > 0 && p.SaisiPar != p.UserID {
		saisiPar = p.SaisiPar
	}
	res, err := tx.Exec(`INSERT INTO checkouts
		(vehicle_id, user_id, statut, started_at, km_start, motif, notes_depart, check_depart,
		 cle_client, depart_hors_ligne, saisi_par)
		VALUES (?,?,'en_cours',?,?,?,?,?,?,?,?)`,
		p.VehicleID, p.UserID, debut, p.KMStart, p.Motif, p.Notes, p.Check,
		nullIfEmpty(p.CleClient), p.HorsLigne, saisiPar)
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

	// Voir PriseEnCompte : mêmes garanties pour une restitution différée.
	CleClient     string
	RetourDeclare time.Time
	HorsLigne     bool
}

// Restituer clôt une prise en compte et remet le véhicule dans le parc.
func (s *Store) Restituer(r Restitution) (*Checkout, error) {
	if r.CleClient != "" {
		if c, err := s.checkoutParCle("cle_client_retour", r.CleClient); err == nil {
			return c, ErrDejaEnregistre
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

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
		return nil, fmt.Errorf("%w : %s km au retour, contre %s km au départ",
			ErrKMIncoherent, FmtKM(r.KMEnd), FmtKM(kmStart))
	}
	if !r.Force && r.KMEnd-kmStart > KMDeltaMax {
		return nil, fmt.Errorf("%w : %s km parcourus sur une seule sortie (maximum %s sans confirmation)",
			ErrKMIncoherent, FmtKM(r.KMEnd-kmStart), FmtKM(KMDeltaMax))
	}

	if r.Check == "" {
		r.Check = "{}"
	}
	retour, err := horodatageDeclare(r.RetourDeclare)
	if err != nil {
		return nil, err
	}
	// Une restitution ne peut pas précéder la sortie qu'elle clôt, même si
	// l'horloge de l'appareil qui l'a saisie est déréglée.
	if retour < debutDe(tx, r.CheckoutID) {
		retour = time.Now().UTC().Format(time.RFC3339)
	}
	var cloture any
	if r.ClotureePar > 0 {
		cloture = r.ClotureePar
	}
	if _, err := tx.Exec(`UPDATE checkouts SET statut='termine', ended_at=?, km_end=?,
		notes_retour=?, check_retour=?, cloture_par=?, cle_client_retour=?,
		retour_hors_ligne=?, retour_enregistre_at=? WHERE id=?`,
		retour, r.KMEnd, r.Notes, r.Check, cloture, nullIfEmpty(r.CleClient),
		r.HorsLigne, time.Now().UTC().Format(time.RFC3339), r.CheckoutID); err != nil {
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

// CheckoutParCleClient retrouve une prise en compte déjà reçue. Sert à répondre
// à un rejeu avant toute autre validation : sans cela, un client qui réessaie
// se heurterait aux gardes métier (« vous détenez déjà un véhicule ») et
// resterait bloqué à rejouer une opération pourtant déjà enregistrée.
func (s *Store) CheckoutParCleClient(cle string) (*Checkout, error) {
	return s.checkoutParCle("cle_client", cle)
}

// checkoutParCle retrouve une opération déjà reçue à partir de sa clé client.
// Le nom de colonne provient exclusivement d'appels internes, jamais d'une
// entrée utilisateur.
func (s *Store) checkoutParCle(colonne, cle string) (*Checkout, error) {
	return scanCheckout(s.DB.QueryRow(
		`SELECT `+checkoutJoinCols+checkoutJoin+` WHERE c.`+colonne+` = ?`, cle), true)
}

// EcartHorlogeMax borne la confiance accordée à l'horloge de l'appareil qui a
// saisi une opération hors ligne.
const (
	EcartHorlogeMax = 5 * time.Minute    // tolérance pour une horloge en avance
	AncienneteMax   = 7 * 24 * time.Hour // au-delà, la saisie est trop vieille
)

// horodatageDeclare valide l'heure fournie par un appareil et la met en forme.
// Une date absente donne l'heure du serveur : c'est le cas d'une saisie en
// ligne, où les deux coïncident de toute façon.
func horodatageDeclare(t time.Time) (string, error) {
	maintenant := time.Now().UTC()
	if t.IsZero() {
		return maintenant.Format(time.RFC3339), nil
	}
	t = t.UTC()
	if t.After(maintenant.Add(EcartHorlogeMax)) {
		return "", fmt.Errorf("%w : l'heure déclarée est dans le futur, vérifiez l'horloge de l'appareil",
			ErrHorodatageInvalide)
	}
	if t.Before(maintenant.Add(-AncienneteMax)) {
		return "", fmt.Errorf("%w : l'opération date de plus de %d jours",
			ErrHorodatageInvalide, int(AncienneteMax.Hours()/24))
	}
	return t.Format(time.RFC3339), nil
}

// debutDe lit l'heure de sortie d'une prise en compte, dans la transaction en
// cours. Renvoie une chaîne vide si elle est illisible, ce qui laisse alors la
// comparaison sans effet.
func debutDe(tx *sql.Tx, checkoutID int64) string {
	var debut string
	if err := tx.QueryRow(`SELECT started_at FROM checkouts WHERE id = ?`, checkoutID).
		Scan(&debut); err != nil {
		return ""
	}
	return debut
}
