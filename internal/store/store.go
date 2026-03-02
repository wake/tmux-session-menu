package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Group struct {
	ID        int64
	Name      string
	SortOrder int
	Collapsed bool
}

type SessionMeta struct {
	SessionName string
	GroupID     int64
	SortOrder   int
	CustomName  string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		sort_order INTEGER NOT NULL DEFAULT 0,
		collapsed INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS session_meta (
		session_name TEXT PRIMARY KEY,
		group_id INTEGER NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL DEFAULT 0,
		custom_name TEXT NOT NULL DEFAULT ''
	);`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) CreateGroup(name string, sortOrder int) error {
	_, err := s.db.Exec("INSERT INTO groups (name, sort_order) VALUES (?, ?)", name, sortOrder)
	return err
}

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query("SELECT id, name, sort_order, collapsed FROM groups ORDER BY sort_order, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.Collapsed); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) RenameGroup(id int64, name string) error {
	_, err := s.db.Exec("UPDATE groups SET name = ? WHERE id = ?", name, id)
	return err
}

func (s *Store) DeleteGroup(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE session_meta SET group_id = 0 WHERE group_id = ?", id); err != nil {
		return fmt.Errorf("ungroup sessions: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM groups WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return tx.Commit()
}

func (s *Store) SetSessionGroup(sessionName string, groupID int64, sortOrder int) error {
	_, err := s.db.Exec(`
		INSERT INTO session_meta (session_name, group_id, sort_order)
		VALUES (?, ?, ?)
		ON CONFLICT(session_name) DO UPDATE SET group_id = ?, sort_order = ?`,
		sessionName, groupID, sortOrder, groupID, sortOrder)
	return err
}

func (s *Store) ListSessionMetas(groupID int64) ([]SessionMeta, error) {
	rows, err := s.db.Query(
		"SELECT session_name, group_id, sort_order, custom_name FROM session_meta WHERE group_id = ? ORDER BY sort_order, session_name",
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metas []SessionMeta
	for rows.Next() {
		var m SessionMeta
		if err := rows.Scan(&m.SessionName, &m.GroupID, &m.SortOrder, &m.CustomName); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

func (s *Store) ToggleGroupCollapsed(id int64) error {
	_, err := s.db.Exec("UPDATE groups SET collapsed = NOT collapsed WHERE id = ?", id)
	return err
}

func (s *Store) ListAllSessionMetas() ([]SessionMeta, error) {
	rows, err := s.db.Query(
		"SELECT session_name, group_id, sort_order, custom_name FROM session_meta ORDER BY sort_order, session_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metas []SessionMeta
	for rows.Next() {
		var m SessionMeta
		if err := rows.Scan(&m.SessionName, &m.GroupID, &m.SortOrder, &m.CustomName); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

func (s *Store) SetGroupOrder(id int64, sortOrder int) error {
	_, err := s.db.Exec("UPDATE groups SET sort_order = ? WHERE id = ?", sortOrder, id)
	return err
}

// SetCustomName 設定 session 的自訂顯示名稱（UPSERT）。
func (s *Store) SetCustomName(sessionName, customName string) error {
	_, err := s.db.Exec(`
		INSERT INTO session_meta (session_name, custom_name)
		VALUES (?, ?)
		ON CONFLICT(session_name) DO UPDATE SET custom_name = ?`,
		sessionName, customName, customName)
	return err
}
