package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection for inventory operations
type DB struct {
	conn *sql.DB
	mu   sync.Mutex

	// Prepared statements - Datacenters
	stmtInsertDC   *sql.Stmt
	stmtUpdateDC   *sql.Stmt
	stmtDeleteDC   *sql.Stmt
	stmtGetDC      *sql.Stmt
	stmtGetDCByName *sql.Stmt
	stmtListDCs    *sql.Stmt

	// Prepared statements - Clusters
	stmtInsertCluster     *sql.Stmt
	stmtUpdateCluster     *sql.Stmt
	stmtDeleteCluster     *sql.Stmt
	stmtGetCluster        *sql.Stmt
	stmtGetClusterByName  *sql.Stmt
	stmtListClusters      *sql.Stmt
	stmtListClustersByDC  *sql.Stmt
	stmtSetClusterEnabled *sql.Stmt
}

// Open creates or opens the inventory database
func Open(dbPath string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000&_foreign_keys=ON", dbPath)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer for SQLite
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := db.prepareStatements(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	slog.Info("inventory database opened", "path", dbPath)
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Close prepared statements - Datacenters
	if db.stmtInsertDC != nil {
		db.stmtInsertDC.Close()
	}
	if db.stmtUpdateDC != nil {
		db.stmtUpdateDC.Close()
	}
	if db.stmtDeleteDC != nil {
		db.stmtDeleteDC.Close()
	}
	if db.stmtGetDC != nil {
		db.stmtGetDC.Close()
	}
	if db.stmtGetDCByName != nil {
		db.stmtGetDCByName.Close()
	}
	if db.stmtListDCs != nil {
		db.stmtListDCs.Close()
	}

	// Close prepared statements - Clusters
	if db.stmtInsertCluster != nil {
		db.stmtInsertCluster.Close()
	}
	if db.stmtUpdateCluster != nil {
		db.stmtUpdateCluster.Close()
	}
	if db.stmtDeleteCluster != nil {
		db.stmtDeleteCluster.Close()
	}
	if db.stmtGetCluster != nil {
		db.stmtGetCluster.Close()
	}
	if db.stmtGetClusterByName != nil {
		db.stmtGetClusterByName.Close()
	}
	if db.stmtListClusters != nil {
		db.stmtListClusters.Close()
	}
	if db.stmtListClustersByDC != nil {
		db.stmtListClustersByDC.Close()
	}
	if db.stmtSetClusterEnabled != nil {
		db.stmtSetClusterEnabled.Close()
	}

	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS datacenters (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS clusters (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		datacenter_id TEXT REFERENCES datacenters(id) ON DELETE SET NULL,
		discovery_node TEXT NOT NULL,
		token_id TEXT NOT NULL,
		insecure INTEGER DEFAULT 1,
		enabled INTEGER DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_clusters_datacenter ON clusters(datacenter_id);
	CREATE INDEX IF NOT EXISTS idx_clusters_enabled ON clusters(enabled);
	`

	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) prepareStatements() error {
	var err error

	// Datacenter statements
	db.stmtInsertDC, err = db.conn.Prepare(`
		INSERT INTO datacenters (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert datacenter: %w", err)
	}

	db.stmtUpdateDC, err = db.conn.Prepare(`
		UPDATE datacenters SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare update datacenter: %w", err)
	}

	db.stmtDeleteDC, err = db.conn.Prepare(`DELETE FROM datacenters WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete datacenter: %w", err)
	}

	db.stmtGetDC, err = db.conn.Prepare(`
		SELECT id, name, description, created_at, updated_at
		FROM datacenters WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get datacenter: %w", err)
	}

	db.stmtGetDCByName, err = db.conn.Prepare(`
		SELECT id, name, description, created_at, updated_at
		FROM datacenters WHERE name = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get datacenter by name: %w", err)
	}

	db.stmtListDCs, err = db.conn.Prepare(`
		SELECT id, name, description, created_at, updated_at
		FROM datacenters ORDER BY name
	`)
	if err != nil {
		return fmt.Errorf("prepare list datacenters: %w", err)
	}

	// Cluster statements
	db.stmtInsertCluster, err = db.conn.Prepare(`
		INSERT INTO clusters (id, name, datacenter_id, discovery_node, token_id, insecure, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert cluster: %w", err)
	}

	db.stmtUpdateCluster, err = db.conn.Prepare(`
		UPDATE clusters SET name = ?, datacenter_id = ?, discovery_node = ?, token_id = ?, insecure = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare update cluster: %w", err)
	}

	db.stmtDeleteCluster, err = db.conn.Prepare(`DELETE FROM clusters WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete cluster: %w", err)
	}

	db.stmtGetCluster, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.datacenter_id, c.discovery_node, c.token_id, c.insecure, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get cluster: %w", err)
	}

	db.stmtGetClusterByName, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.datacenter_id, c.discovery_node, c.token_id, c.insecure, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.name = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get cluster by name: %w", err)
	}

	db.stmtListClusters, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.datacenter_id, c.discovery_node, c.token_id, c.insecure, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		ORDER BY d.name NULLS LAST, c.name
	`)
	if err != nil {
		return fmt.Errorf("prepare list clusters: %w", err)
	}

	db.stmtListClustersByDC, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.datacenter_id, c.discovery_node, c.token_id, c.insecure, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.datacenter_id = ? OR (c.datacenter_id IS NULL AND ? IS NULL)
		ORDER BY c.name
	`)
	if err != nil {
		return fmt.Errorf("prepare list clusters by datacenter: %w", err)
	}

	db.stmtSetClusterEnabled, err = db.conn.Prepare(`
		UPDATE clusters SET enabled = ?, updated_at = ? WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare set cluster enabled: %w", err)
	}

	return nil
}

// === Datacenter Operations ===

// CreateDatacenter creates a new datacenter
func (db *DB) CreateDatacenter(ctx context.Context, req CreateDatacenterRequest) (*Datacenter, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	dc := &Datacenter{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := db.stmtInsertDC.ExecContext(ctx,
		dc.ID, dc.Name, dc.Description,
		dc.CreatedAt.Unix(), dc.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert datacenter: %w", err)
	}

	return dc, nil
}

// GetDatacenter retrieves a datacenter by ID
func (db *DB) GetDatacenter(ctx context.Context, id string) (*Datacenter, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.getDatacenterLocked(ctx, db.stmtGetDC, id)
}

// GetDatacenterByName retrieves a datacenter by name
func (db *DB) GetDatacenterByName(ctx context.Context, name string) (*Datacenter, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.getDatacenterLocked(ctx, db.stmtGetDCByName, name)
}

func (db *DB) getDatacenterLocked(ctx context.Context, stmt *sql.Stmt, arg string) (*Datacenter, error) {
	var dc Datacenter
	var description sql.NullString
	var createdAt, updatedAt int64

	err := stmt.QueryRowContext(ctx, arg).Scan(
		&dc.ID, &dc.Name, &description, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get datacenter: %w", err)
	}

	if description.Valid {
		dc.Description = description.String
	}
	dc.CreatedAt = time.Unix(createdAt, 0)
	dc.UpdatedAt = time.Unix(updatedAt, 0)

	return &dc, nil
}

// ListDatacenters retrieves all datacenters
func (db *DB) ListDatacenters(ctx context.Context) ([]Datacenter, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtListDCs.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("query datacenters: %w", err)
	}
	defer rows.Close()

	var datacenters []Datacenter
	for rows.Next() {
		var dc Datacenter
		var description sql.NullString
		var createdAt, updatedAt int64

		if err := rows.Scan(&dc.ID, &dc.Name, &description, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan datacenter: %w", err)
		}

		if description.Valid {
			dc.Description = description.String
		}
		dc.CreatedAt = time.Unix(createdAt, 0)
		dc.UpdatedAt = time.Unix(updatedAt, 0)

		datacenters = append(datacenters, dc)
	}

	return datacenters, rows.Err()
}

// UpdateDatacenter updates a datacenter
func (db *DB) UpdateDatacenter(ctx context.Context, id string, req UpdateDatacenterRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtUpdateDC.ExecContext(ctx, req.Name, req.Description, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update datacenter: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("datacenter not found")
	}

	return nil
}

// DeleteDatacenter deletes a datacenter (clusters become orphans via ON DELETE SET NULL)
func (db *DB) DeleteDatacenter(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtDeleteDC.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("delete datacenter: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("datacenter not found")
	}

	return nil
}

// === Cluster Operations ===

// CreateCluster creates a new cluster
func (db *DB) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	cluster := &Cluster{
		ID:            uuid.New().String(),
		Name:          req.Name,
		DatacenterID:  req.DatacenterID,
		DiscoveryNode: req.DiscoveryNode,
		TokenID:       req.TokenID,
		Insecure:      req.Insecure,
		Enabled:       true, // enabled by default
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err := db.stmtInsertCluster.ExecContext(ctx,
		cluster.ID, cluster.Name, cluster.DatacenterID,
		cluster.DiscoveryNode, cluster.TokenID,
		boolToInt(cluster.Insecure), boolToInt(cluster.Enabled),
		cluster.CreatedAt.Unix(), cluster.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert cluster: %w", err)
	}

	return cluster, nil
}

// GetCluster retrieves a cluster by ID
func (db *DB) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.getClusterLocked(ctx, db.stmtGetCluster, id)
}

// GetClusterByName retrieves a cluster by name
func (db *DB) GetClusterByName(ctx context.Context, name string) (*Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.getClusterLocked(ctx, db.stmtGetClusterByName, name)
}

func (db *DB) getClusterLocked(ctx context.Context, stmt *sql.Stmt, arg string) (*Cluster, error) {
	var c Cluster
	var datacenterID, datacenterName sql.NullString
	var insecure, enabled int
	var createdAt, updatedAt int64

	err := stmt.QueryRowContext(ctx, arg).Scan(
		&c.ID, &c.Name, &datacenterID, &c.DiscoveryNode, &c.TokenID,
		&insecure, &enabled, &createdAt, &updatedAt, &datacenterName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}

	if datacenterID.Valid {
		c.DatacenterID = &datacenterID.String
	}
	if datacenterName.Valid {
		c.DatacenterName = datacenterName.String
	}
	c.Insecure = insecure != 0
	c.Enabled = enabled != 0
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)

	return &c, nil
}

// ListClusters retrieves all clusters
func (db *DB) ListClusters(ctx context.Context) ([]Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtListClusters.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	return db.scanClusters(rows)
}

// ListClustersByDatacenter retrieves clusters for a datacenter (pass nil for orphans)
func (db *DB) ListClustersByDatacenter(ctx context.Context, datacenterID *string) ([]Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtListClustersByDC.QueryContext(ctx, datacenterID, datacenterID)
	if err != nil {
		return nil, fmt.Errorf("query clusters by datacenter: %w", err)
	}
	defer rows.Close()

	return db.scanClusters(rows)
}

func (db *DB) scanClusters(rows *sql.Rows) ([]Cluster, error) {
	var clusters []Cluster
	for rows.Next() {
		var c Cluster
		var datacenterID, datacenterName sql.NullString
		var insecure, enabled int
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&c.ID, &c.Name, &datacenterID, &c.DiscoveryNode, &c.TokenID,
			&insecure, &enabled, &createdAt, &updatedAt, &datacenterName,
		); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}

		if datacenterID.Valid {
			c.DatacenterID = &datacenterID.String
		}
		if datacenterName.Valid {
			c.DatacenterName = datacenterName.String
		}
		c.Insecure = insecure != 0
		c.Enabled = enabled != 0
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)

		clusters = append(clusters, c)
	}

	return clusters, rows.Err()
}

// UpdateCluster updates a cluster
func (db *DB) UpdateCluster(ctx context.Context, id string, req UpdateClusterRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtUpdateCluster.ExecContext(ctx,
		req.Name, req.DatacenterID, req.DiscoveryNode, req.TokenID,
		boolToInt(req.Insecure), boolToInt(req.Enabled),
		time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update cluster: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("cluster not found")
	}

	return nil
}

// DeleteCluster deletes a cluster
func (db *DB) DeleteCluster(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtDeleteCluster.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("cluster not found")
	}

	return nil
}

// SetClusterEnabled enables or disables a cluster
func (db *DB) SetClusterEnabled(ctx context.Context, id string, enabled bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtSetClusterEnabled.ExecContext(ctx, boolToInt(enabled), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("set cluster enabled: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("cluster not found")
	}

	return nil
}

// === Helpers ===

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ClusterCount returns total cluster count (for migration check)
func (db *DB) ClusterCount(ctx context.Context) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var count int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM clusters").Scan(&count)
	return count, err
}
