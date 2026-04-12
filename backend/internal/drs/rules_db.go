package drs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// RulesDB wraps SQLite for DRS rule storage
type RulesDB struct {
	conn *sql.DB
}

// OpenRulesDB creates or opens the DRS rules database
func OpenRulesDB(dbPath string) (*RulesDB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000&_foreign_keys=ON", dbPath)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	db := &RulesDB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	slog.Info("DRS rules database opened", "path", dbPath)
	return db, nil
}

func (db *RulesDB) Close() error { return db.conn.Close() }

func (db *RulesDB) migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS drs_rules (
			id TEXT PRIMARY KEY,
			cluster TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			members TEXT NOT NULL DEFAULT '[]',
			host_node TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_drs_rules_cluster ON drs_rules(cluster);
	`)
	return err
}

// CreateRule inserts a new rule
func (db *RulesDB) CreateRule(ctx context.Context, rule *Rule) error {
	rule.ID = uuid.New().String()
	members, _ := json.Marshal(rule.Members)
	_, err := db.conn.ExecContext(ctx,
		"INSERT INTO drs_rules (id, cluster, name, type, enabled, members, host_node) VALUES (?, ?, ?, ?, ?, ?, ?)",
		rule.ID, rule.Cluster, rule.Name, rule.Type, rule.Enabled, string(members), rule.HostNode)
	return err
}

// UpdateRule updates an existing rule
func (db *RulesDB) UpdateRule(ctx context.Context, rule *Rule) error {
	members, _ := json.Marshal(rule.Members)
	_, err := db.conn.ExecContext(ctx,
		"UPDATE drs_rules SET name=?, type=?, enabled=?, members=?, host_node=? WHERE id=?",
		rule.Name, rule.Type, rule.Enabled, string(members), rule.HostNode, rule.ID)
	return err
}

// DeleteRule removes a rule
func (db *RulesDB) DeleteRule(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM drs_rules WHERE id=?", id)
	return err
}

// ListRules returns all rules for a cluster
func (db *RulesDB) ListRules(ctx context.Context, cluster string) ([]Rule, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, cluster, name, type, enabled, members, host_node FROM drs_rules WHERE cluster=? ORDER BY name", cluster)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

// ListAllRules returns all rules across all clusters
func (db *RulesDB) ListAllRules(ctx context.Context) ([]Rule, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, cluster, name, type, enabled, members, host_node FROM drs_rules ORDER BY cluster, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

func scanRules(rows *sql.Rows) ([]Rule, error) {
	var rules []Rule
	for rows.Next() {
		var r Rule
		var membersJSON string
		var enabled int
		if err := rows.Scan(&r.ID, &r.Cluster, &r.Name, &r.Type, &enabled, &membersJSON, &r.HostNode); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		json.Unmarshal([]byte(membersJSON), &r.Members)
		if r.Members == nil {
			r.Members = []int{}
		}
		rules = append(rules, r)
	}
	return rules, nil
}
