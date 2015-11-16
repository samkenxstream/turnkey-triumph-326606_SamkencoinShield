package db

import (
	"fmt"
	"github.com/pborman/uuid"
)

type AnnotatedTarget struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Summary  string `json:"summary"`
	Plugin   string `json:"plugin"`
	Endpoint string `json:"endpoint"`
	Agent    string `json:"agent"`
}

type TargetFilter struct {
	SkipUsed   bool
	SkipUnused bool
	ForPlugin  string
}

func (f *TargetFilter) Args() []interface{} {
	args := []interface{}{}
	if f.ForPlugin != "" {
		args = append(args, f.ForPlugin)
	}
	return args
}

func (f *TargetFilter) Query() string {
	where := ""
	if f.ForPlugin != "" {
		where = "WHERE plugin = ?"
	}

	if !f.SkipUsed && !f.SkipUnused {
		return `
			SELECT uuid, name, summary, plugin, endpoint, agent, -1 AS n
				FROM targets ` + where + `
				ORDER BY name, uuid ASC
		`
	}

	// by default, show targets with no attached jobs (unused)
	having := `HAVING n = 0`
	if f.SkipUnused {
		// otherwise, only show targets that have attached jobs
		having = `HAVING n > 0`
	}

	return `
		SELECT DISTINCT t.uuid, t.name, t.summary, t.plugin, t.endpoint, t.agent, COUNT(j.uuid) AS n
			FROM targets t
				LEFT JOIN jobs j
					ON j.target_uuid = t.uuid
			` + where + ` GROUP BY t.uuid
			` + having + `
			ORDER BY t.name, t.uuid ASC
	`
}

func (db *DB) GetAllAnnotatedTargets(filter *TargetFilter) ([]*AnnotatedTarget, error) {
	l := []*AnnotatedTarget{}
	r, err := db.Query(filter.Query(), filter.Args()...)
	if err != nil {
		return l, err
	}
	defer r.Close()

	for r.Next() {
		ann := &AnnotatedTarget{}
		var n int

		if err = r.Scan(&ann.UUID, &ann.Name, &ann.Summary, &ann.Plugin, &ann.Endpoint, &ann.Agent, &n); err != nil {
			return l, err
		}

		l = append(l, ann)
	}

	return l, nil
}

func (db *DB) GetAnnotatedTarget(id uuid.UUID) (*AnnotatedTarget, error) {
	r, err := db.Query(`
		SELECT DISTINCT t.uuid, t.name, t.summary, t.plugin, t.endpoint
			FROM targets t
				LEFT JOIN jobs j
					ON j.target_uuid = t.uuid
			WHERE t.uuid = ?
	`, id.String())
	if err != nil {
		return nil, err
	}
	defer r.Close()

	if !r.Next() {
		return nil, fmt.Errorf("target not found")
	}

	ann := &AnnotatedTarget{}

	if err = r.Scan(&ann.UUID, &ann.Name, &ann.Summary, &ann.Plugin, &ann.Endpoint); err != nil {
		return nil, err
	}

	return ann, nil
}

func (db *DB) AnnotateTarget(id uuid.UUID, name string, summary string) error {
	return db.Exec(
		`UPDATE targets SET name = ?, summary = ? WHERE uuid = ?`,
		name, summary, id.String(),
	)
}

func (db *DB) CreateTarget(plugin string, endpoint interface{}, agent string) (uuid.UUID, error) {
	id := uuid.NewRandom()
	return id, db.Exec(
		`INSERT INTO targets (uuid, plugin, endpoint, agent) VALUES (?, ?, ?, ?)`,
		id.String(), plugin, endpoint, agent,
	)
}

func (db *DB) UpdateTarget(id uuid.UUID, plugin string, endpoint interface{}, agent string) error {
	return db.Exec(
		`UPDATE targets SET plugin = ?, endpoint = ?, agent = ? WHERE uuid = ?`,
		plugin, endpoint, agent, id.String(),
	)
}

func (db *DB) DeleteTarget(id uuid.UUID) (bool, error) {
	r, err := db.Query(
		`SELECT COUNT(uuid) FROM jobs WHERE jobs.target_uuid = ?`,
		id.String(),
	)
	if err != nil {
		return false, err
	}
	defer r.Close()

	// already deleted?
	if !r.Next() {
		return true, nil
	}

	var numJobs int
	if err = r.Scan(&numJobs); err != nil {
		return false, err
	}

	if numJobs < 0 {
		return false, fmt.Errorf("Target %s is in used by %d (negative) Jobs", id.String(), numJobs)
	}
	if numJobs > 0 {
		return false, nil
	}

	r.Close()
	return true, db.Exec(
		`DELETE FROM targets WHERE uuid = ?`,
		id.String(),
	)
}