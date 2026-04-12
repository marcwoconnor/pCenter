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
	stmtInsertDC    *sql.Stmt
	stmtUpdateDC    *sql.Stmt
	stmtDeleteDC    *sql.Stmt
	stmtGetDC       *sql.Stmt
	stmtGetDCByName *sql.Stmt
	stmtListDCs     *sql.Stmt

	// Prepared statements - Clusters
	stmtInsertCluster     *sql.Stmt
	stmtUpdateCluster     *sql.Stmt
	stmtDeleteCluster     *sql.Stmt
	stmtGetCluster        *sql.Stmt
	stmtGetClusterByName  *sql.Stmt
	stmtListClusters      *sql.Stmt
	stmtSetClusterEnabled *sql.Stmt
	stmtSetClusterStatus  *sql.Stmt

	// Prepared statements - Hosts
	stmtInsertHost       *sql.Stmt
	stmtUpdateHost       *sql.Stmt
	stmtDeleteHost       *sql.Stmt
	stmtGetHost          *sql.Stmt
	stmtListHostsByCluster *sql.Stmt
	stmtSetHostStatus    *sql.Stmt
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

	stmts := []*sql.Stmt{
		db.stmtInsertDC, db.stmtUpdateDC, db.stmtDeleteDC, db.stmtGetDC, db.stmtGetDCByName, db.stmtListDCs,
		db.stmtInsertCluster, db.stmtUpdateCluster, db.stmtDeleteCluster, db.stmtGetCluster,
		db.stmtGetClusterByName, db.stmtListClusters, db.stmtSetClusterEnabled, db.stmtSetClusterStatus,
		db.stmtInsertHost, db.stmtUpdateHost, db.stmtDeleteHost, db.stmtGetHost,
		db.stmtListHostsByCluster, db.stmtSetHostStatus,
	}
	for _, stmt := range stmts {
		if stmt != nil {
			stmt.Close()
		}
	}

	return db.conn.Close()
}

func (db *DB) migrate() error {
	// Schema v2: clusters are containers, hosts have connection details
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
		agent_name TEXT,
		datacenter_id TEXT REFERENCES datacenters(id) ON DELETE SET NULL,
		status TEXT DEFAULT 'empty',
		enabled INTEGER DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS inventory_hosts (
		id TEXT PRIMARY KEY,
		cluster_id TEXT REFERENCES clusters(id) ON DELETE CASCADE,
		address TEXT NOT NULL,
		token_id TEXT NOT NULL,
		insecure INTEGER DEFAULT 1,
		status TEXT DEFAULT 'staged',
		error TEXT,
		node_name TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_clusters_datacenter ON clusters(datacenter_id);
	CREATE INDEX IF NOT EXISTS idx_clusters_enabled ON clusters(enabled);
	CREATE INDEX IF NOT EXISTS idx_hosts_cluster ON inventory_hosts(cluster_id);
	`

	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add agent_name column if missing (must happen before index creation)
	var hasAgentName bool
	row := db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('clusters') WHERE name='agent_name'")
	var agentNameCount int
	if err := row.Scan(&agentNameCount); err == nil && agentNameCount > 0 {
		hasAgentName = true
	}
	if !hasAgentName {
		slog.Info("adding agent_name column to clusters")
		db.conn.Exec("ALTER TABLE clusters ADD COLUMN agent_name TEXT")
	}

	// Set agent_name = name for any clusters missing it
	db.conn.Exec("UPDATE clusters SET agent_name = name WHERE agent_name IS NULL OR agent_name = ''")

	// Create agent_name index after column exists
	db.conn.Exec("CREATE INDEX IF NOT EXISTS idx_clusters_agent_name ON clusters(agent_name)")

	// Migration: if old schema had discovery_node column, migrate data to hosts
	var hasDiscoveryNode bool
	row = db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('clusters') WHERE name='discovery_node'")
	var count int
	if err := row.Scan(&count); err == nil && count > 0 {
		hasDiscoveryNode = true
	}

	if hasDiscoveryNode {
		slog.Info("migrating old cluster schema to new host-based schema")
		// Migrate existing clusters to hosts
		rows, err := db.conn.Query(`
			SELECT id, discovery_node, token_id, insecure FROM clusters
			WHERE discovery_node IS NOT NULL AND discovery_node != ''
		`)
		if err != nil {
			return fmt.Errorf("query old clusters: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var clusterID, discoveryNode, tokenID string
			var insecure int
			if err := rows.Scan(&clusterID, &discoveryNode, &tokenID, &insecure); err != nil {
				continue
			}

			// Check if host already exists
			var existingCount int
			db.conn.QueryRow("SELECT COUNT(*) FROM inventory_hosts WHERE cluster_id = ? AND address = ?",
				clusterID, discoveryNode).Scan(&existingCount)
			if existingCount > 0 {
				continue
			}

			now := time.Now().Unix()
			_, err := db.conn.Exec(`
				INSERT INTO inventory_hosts (id, cluster_id, address, token_id, insecure, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, 'online', ?, ?)
			`, uuid.New().String(), clusterID, discoveryNode, tokenID, insecure, now, now)
			if err != nil {
				slog.Warn("failed to migrate cluster to host", "cluster_id", clusterID, "error", err)
			} else {
				// Update cluster status to active
				db.conn.Exec("UPDATE clusters SET status = 'active' WHERE id = ?", clusterID)
				slog.Info("migrated cluster host", "cluster_id", clusterID, "address", discoveryNode)
			}
		}

		// Drop old columns (SQLite doesn't support DROP COLUMN easily, so we'll just ignore them)
		slog.Info("old schema migration complete")
	}

	// Migration: add datacenter_id column to hosts for standalone hosts
	var hasDatacenterID bool
	row = db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('inventory_hosts') WHERE name='datacenter_id'")
	var dcCount int
	if err := row.Scan(&dcCount); err == nil && dcCount > 0 {
		hasDatacenterID = true
	}
	if !hasDatacenterID {
		slog.Info("adding datacenter_id column to inventory_hosts for standalone hosts")
		db.conn.Exec("ALTER TABLE inventory_hosts ADD COLUMN datacenter_id TEXT REFERENCES datacenters(id) ON DELETE CASCADE")
		db.conn.Exec("CREATE INDEX IF NOT EXISTS idx_hosts_datacenter ON inventory_hosts(datacenter_id)")
	}

	// Migration: add token_secret column to inventory_hosts
	var hasTokenSecret bool
	row = db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('inventory_hosts') WHERE name='token_secret'")
	var tsCount int
	if err := row.Scan(&tsCount); err == nil && tsCount > 0 {
		hasTokenSecret = true
	}
	if !hasTokenSecret {
		slog.Info("adding token_secret column to inventory_hosts")
		db.conn.Exec("ALTER TABLE inventory_hosts ADD COLUMN token_secret TEXT DEFAULT ''")
	}

	return nil
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

	// Cluster statements (simplified - no connection details)
	db.stmtInsertCluster, err = db.conn.Prepare(`
		INSERT INTO clusters (id, name, agent_name, datacenter_id, status, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert cluster: %w", err)
	}

	db.stmtUpdateCluster, err = db.conn.Prepare(`
		UPDATE clusters SET name = ?, datacenter_id = ?, enabled = ?, updated_at = ?
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
		SELECT c.id, c.name, c.agent_name, c.datacenter_id, c.status, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get cluster: %w", err)
	}

	db.stmtGetClusterByName, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.agent_name, c.datacenter_id, c.status, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.name = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get cluster by name: %w", err)
	}

	db.stmtListClusters, err = db.conn.Prepare(`
		SELECT c.id, c.name, c.agent_name, c.datacenter_id, c.status, c.enabled, c.created_at, c.updated_at,
		       d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		ORDER BY d.name NULLS LAST, c.name
	`)
	if err != nil {
		return fmt.Errorf("prepare list clusters: %w", err)
	}

	db.stmtSetClusterEnabled, err = db.conn.Prepare(`
		UPDATE clusters SET enabled = ?, updated_at = ? WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare set cluster enabled: %w", err)
	}

	db.stmtSetClusterStatus, err = db.conn.Prepare(`
		UPDATE clusters SET status = ?, updated_at = ? WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare set cluster status: %w", err)
	}

	// Host statements
	db.stmtInsertHost, err = db.conn.Prepare(`
		INSERT INTO inventory_hosts (id, cluster_id, address, token_id, insecure, status, error, node_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert host: %w", err)
	}

	db.stmtUpdateHost, err = db.conn.Prepare(`
		UPDATE inventory_hosts SET address = ?, token_id = ?, insecure = ?, updated_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare update host: %w", err)
	}

	db.stmtDeleteHost, err = db.conn.Prepare(`DELETE FROM inventory_hosts WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete host: %w", err)
	}

	db.stmtGetHost, err = db.conn.Prepare(`
		SELECT id, cluster_id, address, token_id, COALESCE(token_secret, ''), insecure, status, error, node_name, created_at, updated_at
		FROM inventory_hosts WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get host: %w", err)
	}

	db.stmtListHostsByCluster, err = db.conn.Prepare(`
		SELECT id, cluster_id, address, token_id, insecure, status, error, node_name, created_at, updated_at
		FROM inventory_hosts WHERE cluster_id = ? ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("prepare list hosts by cluster: %w", err)
	}

	db.stmtSetHostStatus, err = db.conn.Prepare(`
		UPDATE inventory_hosts SET status = ?, error = ?, node_name = ?, updated_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare set host status: %w", err)
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
		ID:           uuid.New().String(),
		Name:         req.Name,
		AgentName:    req.Name, // Initially agent_name = name
		DatacenterID: req.DatacenterID,
		Status:       ClusterStatusEmpty,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.stmtInsertCluster.ExecContext(ctx,
		cluster.ID, cluster.Name, cluster.AgentName, cluster.DatacenterID,
		string(cluster.Status), boolToInt(cluster.Enabled),
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
	var agentName, datacenterID, datacenterName sql.NullString
	var status string
	var enabled int
	var createdAt, updatedAt int64

	err := stmt.QueryRowContext(ctx, arg).Scan(
		&c.ID, &c.Name, &agentName, &datacenterID, &status,
		&enabled, &createdAt, &updatedAt, &datacenterName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}

	if agentName.Valid {
		c.AgentName = agentName.String
	}
	if datacenterID.Valid {
		c.DatacenterID = &datacenterID.String
	}
	if datacenterName.Valid {
		c.DatacenterName = datacenterName.String
	}
	c.Status = ClusterStatus(status)
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

// ListClustersByDatacenter returns clusters belonging to a datacenter
func (db *DB) ListClustersByDatacenter(ctx context.Context, datacenterID string) ([]Cluster, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.conn.QueryContext(ctx,
		`SELECT c.id, c.name, c.agent_name, c.datacenter_id, c.status, c.enabled, c.created_at, c.updated_at,
			d.name as datacenter_name
		FROM clusters c
		LEFT JOIN datacenters d ON c.datacenter_id = d.id
		WHERE c.datacenter_id = ?
		ORDER BY c.name`, datacenterID)
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
		var agentName, datacenterID, datacenterName sql.NullString
		var status string
		var enabled int
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&c.ID, &c.Name, &agentName, &datacenterID, &status,
			&enabled, &createdAt, &updatedAt, &datacenterName,
		); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}

		if agentName.Valid {
			c.AgentName = agentName.String
		}
		if datacenterID.Valid {
			c.DatacenterID = &datacenterID.String
		}
		if datacenterName.Valid {
			c.DatacenterName = datacenterName.String
		}
		c.Status = ClusterStatus(status)
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
		req.Name, req.DatacenterID, boolToInt(req.Enabled),
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

// DeleteCluster deletes a cluster (hosts cascade delete)
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

// SetClusterStatus updates a cluster's status
func (db *DB) SetClusterStatus(ctx context.Context, id string, status ClusterStatus) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtSetClusterStatus.ExecContext(ctx, string(status), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("set cluster status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("cluster not found")
	}

	return nil
}

// === Host Operations ===

// AddHost adds a host to a cluster
func (db *DB) AddHost(ctx context.Context, clusterID string, req AddHostRequest) (*InventoryHost, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	host := &InventoryHost{
		ID:        uuid.New().String(),
		ClusterID: clusterID,
		Address:   req.Address,
		TokenID:   req.TokenID,
		Insecure:  req.Insecure,
		Status:    HostStatusStaged,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := db.stmtInsertHost.ExecContext(ctx,
		host.ID, host.ClusterID, host.Address, host.TokenID,
		boolToInt(host.Insecure), string(host.Status), "", "",
		host.CreatedAt.Unix(), host.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert host: %w", err)
	}

	return host, nil
}

// AddDatacenterHost adds a standalone host directly to a datacenter (not in a cluster)
func (db *DB) AddDatacenterHost(ctx context.Context, datacenterID string, req AddHostRequest) (*InventoryHost, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	host := &InventoryHost{
		ID:           uuid.New().String(),
		DatacenterID: datacenterID,
		Address:      req.Address,
		TokenID:      req.TokenID,
		TokenSecret:  req.TokenSecret,
		Insecure:     req.Insecure,
		Status:       HostStatusStaged,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO inventory_hosts (id, cluster_id, datacenter_id, address, token_id, token_secret, insecure, status, error, node_name, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, ?, ?, ?, '', '', ?, ?)
	`, host.ID, host.DatacenterID, host.Address, host.TokenID, host.TokenSecret,
		boolToInt(host.Insecure), string(host.Status),
		host.CreatedAt.Unix(), host.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert datacenter host: %w", err)
	}

	return host, nil
}

// ListHostsByDatacenter retrieves standalone hosts for a datacenter
func (db *DB) ListHostsByDatacenter(ctx context.Context, datacenterID string) ([]InventoryHost, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, COALESCE(cluster_id, ''), COALESCE(datacenter_id, ''), address, token_id, COALESCE(token_secret, ''), insecure, status, error, node_name, created_at, updated_at
		FROM inventory_hosts
		WHERE datacenter_id = ? AND (cluster_id IS NULL OR cluster_id = '')
		ORDER BY created_at
	`, datacenterID)
	if err != nil {
		return nil, fmt.Errorf("list datacenter hosts: %w", err)
	}
	defer rows.Close()

	var hosts []InventoryHost
	for rows.Next() {
		var h InventoryHost
		var insecure int
		var status string
		var errMsg, nodeName sql.NullString
		var createdAt, updatedAt int64

		if err := rows.Scan(&h.ID, &h.ClusterID, &h.DatacenterID, &h.Address, &h.TokenID, &h.TokenSecret, &insecure,
			&status, &errMsg, &nodeName, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
		}

		h.Insecure = insecure != 0
		h.Status = HostStatus(status)
		if errMsg.Valid {
			h.Error = errMsg.String
		}
		if nodeName.Valid {
			h.NodeName = nodeName.String
		}
		h.CreatedAt = time.Unix(createdAt, 0)
		h.UpdatedAt = time.Unix(updatedAt, 0)
		hosts = append(hosts, h)
	}

	return hosts, nil
}

// GetHost retrieves a host by ID
func (db *DB) GetHost(ctx context.Context, id string) (*InventoryHost, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var h InventoryHost
	var insecure int
	var status string
	var clusterID sql.NullString
	var errMsg, nodeName sql.NullString
	var createdAt, updatedAt int64

	err := db.stmtGetHost.QueryRowContext(ctx, id).Scan(
		&h.ID, &clusterID, &h.Address, &h.TokenID, &h.TokenSecret, &insecure,
		&status, &errMsg, &nodeName, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get host: %w", err)
	}

	if clusterID.Valid {
		h.ClusterID = clusterID.String
	}
	h.Insecure = insecure != 0
	h.Status = HostStatus(status)
	if errMsg.Valid {
		h.Error = errMsg.String
	}
	if nodeName.Valid {
		h.NodeName = nodeName.String
	}
	h.CreatedAt = time.Unix(createdAt, 0)
	h.UpdatedAt = time.Unix(updatedAt, 0)

	return &h, nil
}

// ListHostsByCluster retrieves hosts for a cluster
func (db *DB) ListHostsByCluster(ctx context.Context, clusterID string) ([]InventoryHost, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtListHostsByCluster.QueryContext(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("query hosts: %w", err)
	}
	defer rows.Close()

	var hosts []InventoryHost
	for rows.Next() {
		var h InventoryHost
		var insecure int
		var status string
		var errMsg, nodeName sql.NullString
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&h.ID, &h.ClusterID, &h.Address, &h.TokenID, &insecure,
			&status, &errMsg, &nodeName, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
		}

		h.Insecure = insecure != 0
		h.Status = HostStatus(status)
		if errMsg.Valid {
			h.Error = errMsg.String
		}
		if nodeName.Valid {
			h.NodeName = nodeName.String
		}
		h.CreatedAt = time.Unix(createdAt, 0)
		h.UpdatedAt = time.Unix(updatedAt, 0)

		hosts = append(hosts, h)
	}

	return hosts, rows.Err()
}

// UpdateHost updates a host's connection details
func (db *DB) UpdateHost(ctx context.Context, id string, req UpdateHostRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtUpdateHost.ExecContext(ctx,
		req.Address, req.TokenID, boolToInt(req.Insecure),
		time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update host: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("host not found")
	}

	return nil
}

// DeleteHost deletes a host
func (db *DB) DeleteHost(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtDeleteHost.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("delete host: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("host not found")
	}

	return nil
}

// SetHostStatus updates a host's status and optionally error/nodename
func (db *DB) SetHostStatus(ctx context.Context, id string, status HostStatus, errMsg, nodeName string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtSetHostStatus.ExecContext(ctx,
		string(status), errMsg, nodeName, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("set host status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("host not found")
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

// ClusterCount returns total cluster count
func (db *DB) ClusterCount(ctx context.Context) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var count int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM clusters").Scan(&count)
	return count, err
}

// HostCountByCluster returns host count for a cluster
func (db *DB) HostCountByCluster(ctx context.Context, clusterID string) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var count int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_hosts WHERE cluster_id = ?", clusterID).Scan(&count)
	return count, err
}
