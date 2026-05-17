package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusPending   = "Aguardando"
	StatusRunning   = "Em Execucao"
	StatusRedoing   = "Refazendo"
	StatusApproved  = "Aprovado"
	StatusRejected  = "Reprovado"
	StatusBlocked   = "Bloqueado"
	StatusCancelled = "Cancelado"
)

type Store struct {
	db *sql.DB
}

type Contract struct {
	ID           int64     `json:"id"`
	NorthStar    string    `json:"north_star"`
	Constraints  string    `json:"constraints"`
	Deliverables string    `json:"deliverables"`
	Hash         string    `json:"hash"`
	Sealed       bool      `json:"sealed"`
	CreatedAt    time.Time `json:"created_at"`
}

type Task struct {
	ID           int64          `json:"id"`
	ContractID   int64          `json:"contract_id"`
	Title        string         `json:"title"`
	Role         string         `json:"role"`
	Dependencies []int64        `json:"dependencies"`
	Status       string         `json:"status"`
	Attempts     int            `json:"attempts"`
	ReadOnly     bool           `json:"read_only"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Agent struct {
	ID        int64     `json:"id"`
	WorkerID  int       `json:"worker_id"`
	Role      string    `json:"role"`
	TaskID    int64     `json:"task_id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LogEntry struct {
	ID        int64     `json:"id"`
	Level     string    `json:"level"`
	Scope     string    `json:"scope"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type KnowledgeItem struct {
	ID         int64     `json:"id"`
	Topic      string    `json:"topic"`
	Rules      []byte    `json:"rules"`
	Format     string    `json:"format"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type KnowledgeCandidate struct {
	ID         int64     `json:"id"`
	ContractID int64     `json:"contract_id"`
	TaskID     int64     `json:"task_id"`
	Domain     string    `json:"domain"`
	Pattern    string    `json:"pattern"`
	Action     string    `json:"action"`
	Template   string    `json:"template"`
	Evidence   string    `json:"evidence"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
}

type Interrogation struct {
	ID           int64             `json:"id"`
	Input        string            `json:"input"`
	Questions    []string          `json:"questions"`
	Answers      map[string]string `json:"answers"`
	NorthStar    string            `json:"north_star"`
	Constraints  string            `json:"constraints"`
	Deliverables string            `json:"deliverables"`
	Status       string            `json:"status"`
	ContractID   int64             `json:"contract_id"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type FactoryBatch struct {
	ID           int64     `json:"id"`
	BatchID      string    `json:"batch_id"`
	Template     string    `json:"template"`
	Items        []string  `json:"items"`
	Constraints  string    `json:"constraints"`
	Deliverables string    `json:"deliverables"`
	Status       string    `json:"status"`
	Total        int       `json:"total"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type FactoryItem struct {
	ID         int64     `json:"id"`
	BatchID    string    `json:"batch_id"`
	Index      int       `json:"index"`
	Item       string    `json:"item"`
	ContractID int64     `json:"contract_id"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Artifact struct {
	ID         int64     `json:"id"`
	ContractID int64     `json:"contract_id"`
	TaskID     int64     `json:"task_id"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	ReadOnly   bool      `json:"read_only"`
	CreatedAt  time.Time `json:"created_at"`
}

type Handoff struct {
	ID         int64          `json:"id"`
	ContractID int64          `json:"contract_id"`
	FromTaskID int64          `json:"from_task_id"`
	ToTaskID   int64          `json:"to_task_id"`
	Kind       string         `json:"kind"`
	Payload    map[string]any `json:"payload"`
	SHA256     string         `json:"sha256"`
	CreatedAt  time.Time      `json:"created_at"`
}

type PluginCall struct {
	ID         int64     `json:"id"`
	PluginID   string    `json:"plugin_id"`
	Tool       string    `json:"tool"`
	Transport  string    `json:"transport,omitempty"`
	ContractID int64     `json:"contract_id,omitempty"`
	TaskID     int64     `json:"task_id,omitempty"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	OK         bool      `json:"ok"`
	Duration   string    `json:"duration"`
	Sandboxed  bool      `json:"sandboxed"`
	WorkDir    string    `json:"work_dir,omitempty"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type PluginCommandRegistration struct {
	ID           int64     `json:"id"`
	PluginID     string    `json:"plugin_id"`
	Tool         string    `json:"tool"`
	Kind         string    `json:"kind"`
	Transport    string    `json:"transport"`
	Target       string    `json:"target"`
	Enabled      bool      `json:"enabled"`
	ManifestPath string    `json:"manifest_path"`
	Sandbox      string    `json:"sandbox"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type PluginPermissionScope struct {
	ID          int64     `json:"id"`
	Scope       string    `json:"scope"`
	ContractID  int64     `json:"contract_id,omitempty"`
	TaskID      int64     `json:"task_id,omitempty"`
	PluginID    string    `json:"plugin_id"`
	Tool        string    `json:"tool"`
	Permissions string    `json:"permissions"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PluginCallRecord struct {
	PluginID   string
	Tool       string
	Transport  string
	ContractID int64
	TaskID     int64
	Input      any
	Output     string
	OK         bool
	Duration   string
	Sandboxed  bool
	WorkDir    string
	Error      string
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	store := &Store{db: db}
	if err := store.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	return s.db.Ping()
}

func (s *Store) Migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS contratos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			north_star TEXT NOT NULL,
			constraints TEXT NOT NULL,
			deliverables TEXT NOT NULL,
			hash TEXT NOT NULL UNIQUE,
			sealed INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS tarefas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			role TEXT NOT NULL,
			dependencies TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			read_only INTEGER NOT NULL DEFAULT 0,
			payload TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(contract_id) REFERENCES contratos(id)
		);`,
		`CREATE TABLE IF NOT EXISTS agentes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			worker_id INTEGER NOT NULL UNIQUE,
			role TEXT NOT NULL,
			task_id INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS knowledge_packs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL UNIQUE,
			rules BLOB NOT NULL,
			format TEXT NOT NULL DEFAULT 'gob-rulemap',
			expires_at DATETIME NOT NULL,
			last_used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS knowledge_candidates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			task_id INTEGER NOT NULL,
			domain TEXT NOT NULL,
			pattern TEXT NOT NULL,
			action TEXT NOT NULL,
			template TEXT NOT NULL,
			evidence TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			approved_at DATETIME,
			UNIQUE(task_id, action),
			FOREIGN KEY(contract_id) REFERENCES contratos(id),
			FOREIGN KEY(task_id) REFERENCES tarefas(id)
		);`,
		`CREATE TABLE IF NOT EXISTS logs_tecnicos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT NOT NULL,
			scope TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sistema (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS interrogatorios (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			input TEXT NOT NULL,
			questions TEXT NOT NULL DEFAULT '[]',
			answers TEXT NOT NULL DEFAULT '{}',
			north_star TEXT NOT NULL,
			constraints TEXT NOT NULL,
			deliverables TEXT NOT NULL,
			status TEXT NOT NULL,
			contract_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS factory_batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT NOT NULL UNIQUE,
			template TEXT NOT NULL,
			items TEXT NOT NULL DEFAULT '[]',
			constraints TEXT NOT NULL,
			deliverables TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			total INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS factory_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT NOT NULL,
			item_index INTEGER NOT NULL,
			item TEXT NOT NULL,
			contract_id INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(batch_id, item_index),
			FOREIGN KEY(batch_id) REFERENCES factory_batches(batch_id),
			FOREIGN KEY(contract_id) REFERENCES contratos(id)
		);`,
		`CREATE TABLE IF NOT EXISTS artefatos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			task_id INTEGER NOT NULL,
			path TEXT NOT NULL UNIQUE,
			sha256 TEXT NOT NULL,
			read_only INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(contract_id) REFERENCES contratos(id),
			FOREIGN KEY(task_id) REFERENCES tarefas(id)
		);`,
		`CREATE TABLE IF NOT EXISTS handoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id INTEGER NOT NULL,
			from_task_id INTEGER NOT NULL,
			to_task_id INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL,
			payload TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(contract_id) REFERENCES contratos(id),
			FOREIGN KEY(from_task_id) REFERENCES tarefas(id)
		);`,
		`CREATE TABLE IF NOT EXISTS plugin_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plugin_id TEXT NOT NULL,
			tool TEXT NOT NULL,
			transport TEXT NOT NULL DEFAULT '',
			contract_id INTEGER NOT NULL DEFAULT 0,
			task_id INTEGER NOT NULL DEFAULT 0,
			input TEXT NOT NULL,
			output TEXT NOT NULL,
			ok INTEGER NOT NULL,
			duration TEXT NOT NULL,
			sandboxed INTEGER NOT NULL DEFAULT 0,
			work_dir TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS plugin_command_registry (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plugin_id TEXT NOT NULL,
			tool TEXT NOT NULL,
			kind TEXT NOT NULL,
			transport TEXT NOT NULL,
			target TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			manifest_path TEXT NOT NULL,
			sandbox TEXT NOT NULL DEFAULT '{}',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(plugin_id, tool)
		);`,
		`CREATE TABLE IF NOT EXISTS plugin_permission_scopes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope TEXT NOT NULL,
			contract_id INTEGER NOT NULL DEFAULT 0,
			task_id INTEGER NOT NULL DEFAULT 0,
			plugin_id TEXT NOT NULL DEFAULT '*',
			tool TEXT NOT NULL DEFAULT '*',
			permissions TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(scope, contract_id, task_id, plugin_id, tool)
		);`,
		`CREATE TRIGGER IF NOT EXISTS contratos_immutable_update
			BEFORE UPDATE ON contratos
			WHEN OLD.sealed = 1
			BEGIN
				SELECT RAISE(ABORT, 'contrato imutavel');
			END;`,
		`CREATE TRIGGER IF NOT EXISTS contratos_immutable_delete
			BEFORE DELETE ON contratos
			WHEN OLD.sealed = 1
			BEGIN
				SELECT RAISE(ABORT, 'contrato imutavel');
			END;`,
	}
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := s.migrateLegacyKnowledgeTable(); err != nil {
		return err
	}
	if err := s.migrateLegacyHandoffsTable(); err != nil {
		return err
	}
	if err := s.migratePluginCallsTable(); err != nil {
		return err
	}
	for i := 1; i <= 4; i++ {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO agentes(worker_id, role, task_id, status) VALUES(?, '', 0, ?)`, i, StatusPending); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacyKnowledgeTable() error {
	var exists int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'conhecimento'`).Scan(&exists)
	if err != nil || exists == 0 {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO knowledge_packs(topic, rules, format, expires_at, last_used_at, created_at)
		SELECT topic, rules, 'gob-rulemap', expires_at, last_used_at, created_at FROM conhecimento`)
	return err
}

func (s *Store) migrateLegacyHandoffsTable() error {
	var exists int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'transferencias'`).Scan(&exists)
	if err != nil || exists == 0 {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO handoffs(id, contract_id, from_task_id, to_task_id, kind, payload, sha256, created_at)
		SELECT id, contract_id, from_task_id, to_task_id, kind, payload, sha256, created_at FROM transferencias`)
	return err
}

func (s *Store) migratePluginCallsTable() error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"transport", `ALTER TABLE plugin_calls ADD COLUMN transport TEXT NOT NULL DEFAULT ''`},
		{"contract_id", `ALTER TABLE plugin_calls ADD COLUMN contract_id INTEGER NOT NULL DEFAULT 0`},
		{"task_id", `ALTER TABLE plugin_calls ADD COLUMN task_id INTEGER NOT NULL DEFAULT 0`},
		{"sandboxed", `ALTER TABLE plugin_calls ADD COLUMN sandboxed INTEGER NOT NULL DEFAULT 0`},
		{"work_dir", `ALTER TABLE plugin_calls ADD COLUMN work_dir TEXT NOT NULL DEFAULT ''`},
		{"error", `ALTER TABLE plugin_calls ADD COLUMN error TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		exists, err := s.tableColumnExists("plugin_calls", column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := s.db.Exec(column.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) tableColumnExists(table, column string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func ContractHash(northStar, constraints, deliverables string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(northStar) + "\n---\n" + strings.TrimSpace(constraints) + "\n---\n" + strings.TrimSpace(deliverables)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateContract(northStar, constraints, deliverables string) (Contract, error) {
	hash := ContractHash(northStar, constraints, deliverables)
	res, err := s.db.Exec(`INSERT INTO contratos(north_star, constraints, deliverables, hash, sealed) VALUES(?, ?, ?, ?, 1)`, northStar, constraints, deliverables, hash)
	if err != nil {
		return Contract{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetContract(id)
}

func (s *Store) CreateInterrogation(input string, questions []string, northStar, constraints, deliverables string) (Interrogation, error) {
	rawQuestions, _ := json.Marshal(questions)
	res, err := s.db.Exec(`INSERT INTO interrogatorios(input, questions, north_star, constraints, deliverables, status) VALUES(?, ?, ?, ?, ?, ?)`,
		input, string(rawQuestions), northStar, constraints, deliverables, "Aguardando Respostas")
	if err != nil {
		return Interrogation{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetInterrogation(id)
}

func (s *Store) GetInterrogation(id int64) (Interrogation, error) {
	row := s.db.QueryRow(`SELECT id, input, questions, answers, north_star, constraints, deliverables, status, contract_id, created_at, updated_at FROM interrogatorios WHERE id = ?`, id)
	return scanInterrogation(row)
}

func (s *Store) ListInterrogations() ([]Interrogation, error) {
	rows, err := s.db.Query(`SELECT id, input, questions, answers, north_star, constraints, deliverables, status, contract_id, created_at, updated_at FROM interrogatorios ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Interrogation
	for rows.Next() {
		item, err := scanInterrogation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SealInterrogation(id, contractID int64, answers map[string]string, northStar, constraints, deliverables string) (Interrogation, error) {
	rawAnswers, _ := json.Marshal(answers)
	res, err := s.db.Exec(`UPDATE interrogatorios SET answers = ?, north_star = ?, constraints = ?, deliverables = ?, status = ?, contract_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status != ?`,
		string(rawAnswers), northStar, constraints, deliverables, "Contrato Selado", contractID, id, "Contrato Selado")
	if err != nil {
		return Interrogation{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return Interrogation{}, errors.New("interrogatorio inexistente ou ja selado")
	}
	return s.GetInterrogation(id)
}

func (s *Store) UpsertFactoryBatch(batchID, template string, items []string, constraints, deliverables, status string) (FactoryBatch, error) {
	if strings.TrimSpace(batchID) == "" {
		return FactoryBatch{}, errors.New("batch_id vazio")
	}
	if strings.TrimSpace(status) == "" {
		status = "active"
	}
	raw, _ := json.Marshal(items)
	_, err := s.db.Exec(`INSERT INTO factory_batches(batch_id, template, items, constraints, deliverables, status, total)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(batch_id) DO UPDATE SET
			template = excluded.template,
			items = excluded.items,
			constraints = excluded.constraints,
			deliverables = excluded.deliverables,
			status = excluded.status,
			total = excluded.total,
			updated_at = CURRENT_TIMESTAMP`,
		batchID, template, string(raw), constraints, deliverables, status, len(items))
	if err != nil {
		return FactoryBatch{}, err
	}
	return s.GetFactoryBatch(batchID)
}

func (s *Store) GetFactoryBatch(batchID string) (FactoryBatch, error) {
	row := s.db.QueryRow(`SELECT id, batch_id, template, items, constraints, deliverables, status, total, created_at, updated_at FROM factory_batches WHERE batch_id = ?`, batchID)
	return scanFactoryBatch(row)
}

func (s *Store) ListFactoryBatches(limit int) ([]FactoryBatch, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, batch_id, template, items, constraints, deliverables, status, total, created_at, updated_at FROM factory_batches ORDER BY updated_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactoryBatch
	for rows.Next() {
		item, err := scanFactoryBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SetFactoryBatchStatus(batchID, status string) (FactoryBatch, error) {
	if strings.TrimSpace(batchID) == "" {
		return FactoryBatch{}, errors.New("batch_id vazio")
	}
	if strings.TrimSpace(status) == "" {
		return FactoryBatch{}, errors.New("status do lote vazio")
	}
	res, err := s.db.Exec(`UPDATE factory_batches SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE batch_id = ?`, status, batchID)
	if err != nil {
		return FactoryBatch{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return FactoryBatch{}, errors.New("lote da fabrica inexistente")
	}
	return s.GetFactoryBatch(batchID)
}

func (s *Store) UpsertFactoryItem(batchID string, index int, item string, contractID int64, status string) (FactoryItem, error) {
	if strings.TrimSpace(batchID) == "" {
		return FactoryItem{}, errors.New("batch_id vazio")
	}
	if index <= 0 {
		return FactoryItem{}, errors.New("indice de item invalido")
	}
	if strings.TrimSpace(status) == "" {
		status = "pending"
	}
	_, err := s.db.Exec(`INSERT INTO factory_items(batch_id, item_index, item, contract_id, status, started_at, finished_at)
		VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CASE WHEN ? IN ('created', 'approved', 'blocked', 'cancelled', 'skipped') THEN CURRENT_TIMESTAMP ELSE NULL END)
		ON CONFLICT(batch_id, item_index) DO UPDATE SET
			item = excluded.item,
			contract_id = excluded.contract_id,
			status = excluded.status,
			started_at = COALESCE(factory_items.started_at, excluded.started_at),
			finished_at = CASE WHEN excluded.status IN ('created', 'approved', 'blocked', 'cancelled', 'skipped') THEN CURRENT_TIMESTAMP ELSE factory_items.finished_at END,
			updated_at = CURRENT_TIMESTAMP`,
		batchID, index, item, contractID, status, status)
	if err != nil {
		return FactoryItem{}, err
	}
	return s.GetFactoryItem(batchID, index)
}

func (s *Store) SetFactoryItemStatus(batchID string, index int, status string) (FactoryItem, error) {
	if strings.TrimSpace(batchID) == "" {
		return FactoryItem{}, errors.New("batch_id vazio")
	}
	if index <= 0 {
		return FactoryItem{}, errors.New("indice de item invalido")
	}
	if strings.TrimSpace(status) == "" {
		return FactoryItem{}, errors.New("status do item vazio")
	}
	res, err := s.db.Exec(`UPDATE factory_items SET status = ?, finished_at = CASE WHEN ? IN ('approved', 'blocked', 'cancelled', 'skipped') THEN CURRENT_TIMESTAMP ELSE finished_at END, updated_at = CURRENT_TIMESTAMP WHERE batch_id = ? AND item_index = ?`,
		status, status, batchID, index)
	if err != nil {
		return FactoryItem{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return FactoryItem{}, errors.New("item da fabrica inexistente")
	}
	return s.GetFactoryItem(batchID, index)
}

func (s *Store) GetFactoryItem(batchID string, index int) (FactoryItem, error) {
	row := s.db.QueryRow(`SELECT id, batch_id, item_index, item, contract_id, status, started_at, finished_at, created_at, updated_at FROM factory_items WHERE batch_id = ? AND item_index = ?`, batchID, index)
	return scanFactoryItem(row)
}

func (s *Store) ListFactoryItems(batchID string) ([]FactoryItem, error) {
	rows, err := s.db.Query(`SELECT id, batch_id, item_index, item, contract_id, status, started_at, finished_at, created_at, updated_at FROM factory_items WHERE batch_id = ? ORDER BY item_index`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactoryItem
	for rows.Next() {
		item, err := scanFactoryItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAllFactoryItems(limit int) ([]FactoryItem, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.Query(`SELECT id, batch_id, item_index, item, contract_id, status, started_at, finished_at, created_at, updated_at FROM factory_items ORDER BY updated_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactoryItem
	for rows.Next() {
		item, err := scanFactoryItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CancelFactoryItemTasks(contractID int64, reason string) (int64, error) {
	payload := fmt.Sprintf(`{"cancel_reason":%q}`, reason)
	res, err := s.db.Exec(`UPDATE tarefas SET status = ?, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE contract_id = ? AND read_only = 0 AND status NOT IN (?, ?)`,
		StatusCancelled, payload, contractID, StatusApproved, StatusCancelled)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) GetContract(id int64) (Contract, error) {
	row := s.db.QueryRow(`SELECT id, north_star, constraints, deliverables, hash, sealed, created_at FROM contratos WHERE id = ?`, id)
	var c Contract
	var sealed int
	if err := row.Scan(&c.ID, &c.NorthStar, &c.Constraints, &c.Deliverables, &c.Hash, &sealed, &c.CreatedAt); err != nil {
		return Contract{}, err
	}
	c.Sealed = sealed == 1
	return c, nil
}

func (s *Store) ListContracts() ([]Contract, error) {
	rows, err := s.db.Query(`SELECT id, north_star, constraints, deliverables, hash, sealed, created_at FROM contratos ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contract
	for rows.Next() {
		var c Contract
		var sealed int
		if err := rows.Scan(&c.ID, &c.NorthStar, &c.Constraints, &c.Deliverables, &c.Hash, &sealed, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Sealed = sealed == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddTask(contractID int64, title, role string, deps []int64) (Task, error) {
	return s.AddTaskWithPayload(contractID, title, role, deps, nil)
}

func (s *Store) AddTaskWithPayload(contractID int64, title, role string, deps []int64, payload map[string]any) (Task, error) {
	rawDeps, _ := json.Marshal(deps)
	if payload == nil {
		payload = map[string]any{}
	}
	rawPayload, _ := json.Marshal(payload)
	res, err := s.db.Exec(`INSERT INTO tarefas(contract_id, title, role, dependencies, status, payload) VALUES(?, ?, ?, ?, ?, ?)`, contractID, title, role, string(rawDeps), StatusPending, string(rawPayload))
	if err != nil {
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetTask(id)
}

func (s *Store) AddAuditTask(contractID int64) (Task, error) {
	tasks, err := s.ListTasks()
	if err != nil {
		return Task{}, err
	}
	var deps []int64
	for _, task := range tasks {
		if task.ContractID == contractID && task.Status == StatusApproved {
			deps = append(deps, task.ID)
		}
	}
	return s.AddTask(contractID, "Reexecutar auditoria tecnica do contrato", "Auditor Tecnico", deps)
}

func (s *Store) GetTask(id int64) (Task, error) {
	row := s.db.QueryRow(`SELECT id, contract_id, title, role, dependencies, status, attempts, read_only, payload, created_at, updated_at FROM tarefas WHERE id = ?`, id)
	return scanTask(row)
}

func (s *Store) ReadyTasks(limit int) ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, contract_id, title, role, dependencies, status, attempts, read_only, payload, created_at, updated_at FROM tarefas WHERE status IN (?, ?) AND read_only = 0 ORDER BY id LIMIT ?`, StatusPending, StatusRedoing, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		if s.dependenciesApproved(task.Dependencies) {
			tasks = append(tasks, task)
		}
	}
	return tasks, rows.Err()
}

func (s *Store) ListTasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, contract_id, title, role, dependencies, status, attempts, read_only, payload, created_at, updated_at FROM tarefas ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) ListArtifacts() ([]Artifact, error) {
	rows, err := s.db.Query(`SELECT id, contract_id, task_id, path, sha256, read_only, created_at FROM artefatos ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		var readOnly int
		if err := rows.Scan(&a.ID, &a.ContractID, &a.TaskID, &a.Path, &a.SHA256, &readOnly, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.ReadOnly = readOnly == 1
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

func (s *Store) ArtifactExists(path string) (bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM artefatos WHERE path = ? AND read_only = 1`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) AddArtifact(contractID, taskID int64, path, sha256 string) error {
	_, err := s.db.Exec(`INSERT INTO artefatos(contract_id, task_id, path, sha256, read_only) VALUES(?, ?, ?, ?, 1)`, contractID, taskID, path, sha256)
	return err
}

func (s *Store) AddHandoff(contractID, fromTaskID, toTaskID int64, kind string, payload map[string]any) (Handoff, error) {
	if strings.TrimSpace(kind) == "" {
		kind = "handoff"
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["handoff"] = map[string]any{
		"contract_id":  contractID,
		"from_task_id": fromTaskID,
		"to_task_id":   toTaskID,
		"kind":         kind,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Handoff{}, err
	}
	transfer := map[string]any{
		"contract_id":  contractID,
		"from_task_id": fromTaskID,
		"to_task_id":   toTaskID,
		"kind":         kind,
		"payload":      payload,
	}
	rawTransfer, err := json.Marshal(transfer)
	if err != nil {
		return Handoff{}, err
	}
	sum := sha256.Sum256(rawTransfer)
	hash := hex.EncodeToString(sum[:])
	res, err := s.db.Exec(`INSERT INTO handoffs(contract_id, from_task_id, to_task_id, kind, payload, sha256) VALUES(?, ?, ?, ?, ?, ?)`,
		contractID, fromTaskID, toTaskID, kind, string(raw), hash)
	if err != nil {
		return Handoff{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetHandoff(id)
}

func (s *Store) GetHandoff(id int64) (Handoff, error) {
	row := s.db.QueryRow(`SELECT id, contract_id, from_task_id, to_task_id, kind, payload, sha256, created_at FROM handoffs WHERE id = ?`, id)
	return scanHandoff(row)
}

func (s *Store) ListHandoffs(limit int) ([]Handoff, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, contract_id, from_task_id, to_task_id, kind, payload, sha256, created_at FROM handoffs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var handoffs []Handoff
	for rows.Next() {
		item, err := scanHandoff(rows)
		if err != nil {
			return nil, err
		}
		handoffs = append(handoffs, item)
	}
	return handoffs, rows.Err()
}

func (s *Store) HandoffsForTask(task Task) ([]Handoff, error) {
	if len(task.Dependencies) == 0 {
		return nil, nil
	}
	var handoffs []Handoff
	for _, depID := range task.Dependencies {
		rows, err := s.db.Query(`SELECT id, contract_id, from_task_id, to_task_id, kind, payload, sha256, created_at FROM handoffs WHERE contract_id = ? AND from_task_id = ? AND (to_task_id = 0 OR to_task_id = ?) ORDER BY id`,
			task.ContractID, depID, task.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			item, err := scanHandoff(rows)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			handoffs = append(handoffs, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return handoffs, nil
}

func (s *Store) MarkRunning(taskID int64) error {
	return s.setTaskStatus(taskID, StatusRunning, false)
}

func (s *Store) ApproveTask(taskID int64, payload map[string]any) error {
	raw, _ := json.Marshal(payload)
	res, err := s.db.Exec(`UPDATE tarefas SET status = ?, read_only = 1, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND read_only = 0 AND status = ?`, StatusApproved, string(raw), taskID, StatusRunning)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("tarefa nao esta mais em execucao ou ja foi selada")
	}
	return nil
}

func (s *Store) RejectTask(taskID int64, reason string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	if task.ReadOnly {
		return errors.New("tarefa selada como read-only")
	}
	attempts := task.Attempts + 1
	status := StatusRedoing
	if attempts >= 3 {
		status = StatusBlocked
	}
	payload := map[string]any{
		"reason": reason,
		"watchdog": map[string]any{
			"task_id":   taskID,
			"attempt":   attempts,
			"limit":     3,
			"reason":    reason,
			"blocked":   status == StatusBlocked,
			"status":    status,
			"failed_at": time.Now().Format(time.RFC3339),
		},
	}
	raw, _ := json.Marshal(payload)
	res, err := s.db.Exec(`UPDATE tarefas SET status = ?, attempts = ?, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND read_only = 0`,
		status, attempts, string(raw), taskID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("tarefa inexistente ou selada como read-only")
	}
	return nil
}

func (s *Store) CancelRunning(reason string) error {
	_, err := s.db.Exec(`UPDATE tarefas SET status = ?, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE status = ? AND read_only = 0`, StatusCancelled, fmt.Sprintf(`{"panic_reason":%q}`, reason), StatusRunning)
	return err
}

func (s *Store) CancelTask(taskID int64, reason string) error {
	_, err := s.db.Exec(`UPDATE tarefas SET status = ?, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ? AND read_only = 0`,
		StatusCancelled, fmt.Sprintf(`{"cancel_reason":%q}`, reason), taskID, StatusRunning)
	return err
}

func (s *Store) RetryTask(taskID int64, reason string) error {
	res, err := s.db.Exec(`UPDATE tarefas SET status = ?, payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND read_only = 0 AND status IN (?, ?, ?, ?)`,
		StatusPending, fmt.Sprintf(`{"retry_reason":%q}`, reason), taskID, StatusRejected, StatusBlocked, StatusCancelled, StatusRedoing)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("tarefa nao pode ser refeita; talvez ja esteja selada como read-only")
	}
	return nil
}

func (s *Store) SetAgent(workerID int, role string, taskID int64, status string) error {
	_, err := s.db.Exec(`UPDATE agentes SET role = ?, task_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE worker_id = ?`, role, taskID, status, workerID)
	return err
}

func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT id, worker_id, role, task_id, status, updated_at FROM agentes ORDER BY worker_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.WorkerID, &a.Role, &a.TaskID, &a.Status, &a.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) Log(level, scope, message string) {
	_, _ = s.db.Exec(`INSERT INTO logs_tecnicos(level, scope, message) VALUES(?, ?, ?)`, level, scope, message)
	log.Printf("[%s] %s: %s", level, scope, message)
}

func (s *Store) AddPluginCall(pluginID, tool string, input any, output string, ok bool, duration string) error {
	return s.AddPluginCallRecord(PluginCallRecord{
		PluginID: pluginID,
		Tool:     tool,
		Input:    input,
		Output:   output,
		OK:       ok,
		Duration: duration,
	})
}

func (s *Store) AddPluginCallRecord(record PluginCallRecord) error {
	raw, _ := json.Marshal(record.Input)
	okInt := 0
	if record.OK {
		okInt = 1
	}
	sandboxedInt := 0
	if record.Sandboxed {
		sandboxedInt = 1
	}
	_, err := s.db.Exec(`INSERT INTO plugin_calls(plugin_id, tool, transport, contract_id, task_id, input, output, ok, duration, sandboxed, work_dir, error) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.PluginID, record.Tool, record.Transport, record.ContractID, record.TaskID, string(raw), record.Output, okInt, record.Duration, sandboxedInt, record.WorkDir, record.Error)
	return err
}

func (s *Store) UpsertPluginCommandRegistry(records []PluginCommandRegistration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		enabled := 0
		if record.Enabled {
			enabled = 1
		}
		if strings.TrimSpace(record.Sandbox) == "" {
			record.Sandbox = "{}"
		}
		_, err := tx.Exec(`INSERT INTO plugin_command_registry(plugin_id, tool, kind, transport, target, enabled, manifest_path, sandbox, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(plugin_id, tool) DO UPDATE SET
				kind = excluded.kind,
				transport = excluded.transport,
				target = excluded.target,
				enabled = excluded.enabled,
				manifest_path = excluded.manifest_path,
				sandbox = excluded.sandbox,
				updated_at = CURRENT_TIMESTAMP`,
			record.PluginID, record.Tool, record.Kind, record.Transport, record.Target, enabled, record.ManifestPath, record.Sandbox)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListPluginCommandRegistry(limit int) ([]PluginCommandRegistration, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, plugin_id, tool, kind, transport, target, enabled, manifest_path, sandbox, updated_at FROM plugin_command_registry ORDER BY updated_at DESC, plugin_id, tool LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PluginCommandRegistration
	for rows.Next() {
		var item PluginCommandRegistration
		var enabled int
		if err := rows.Scan(&item.ID, &item.PluginID, &item.Tool, &item.Kind, &item.Transport, &item.Target, &enabled, &item.ManifestPath, &item.Sandbox, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertPluginPermissionScope(scope PluginPermissionScope) error {
	scope.Scope = strings.TrimSpace(scope.Scope)
	if scope.Scope != "contract" && scope.Scope != "task" {
		return errors.New("escopo de permissao deve ser contract ou task")
	}
	if scope.Scope == "contract" && scope.ContractID <= 0 {
		return errors.New("permissao de contrato exige contract_id")
	}
	if scope.Scope == "task" && scope.TaskID <= 0 {
		return errors.New("permissao de tarefa exige task_id")
	}
	if strings.TrimSpace(scope.PluginID) == "" {
		scope.PluginID = "*"
	}
	if strings.TrimSpace(scope.Tool) == "" {
		scope.Tool = "*"
	}
	if strings.TrimSpace(scope.Permissions) == "" {
		scope.Permissions = "{}"
	}
	enabled := 0
	if scope.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`INSERT INTO plugin_permission_scopes(scope, contract_id, task_id, plugin_id, tool, permissions, enabled, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(scope, contract_id, task_id, plugin_id, tool) DO UPDATE SET
			permissions = excluded.permissions,
			enabled = excluded.enabled,
			updated_at = CURRENT_TIMESTAMP`,
		scope.Scope, scope.ContractID, scope.TaskID, scope.PluginID, scope.Tool, scope.Permissions, enabled)
	return err
}

func (s *Store) ListPluginPermissionScopes(limit int) ([]PluginPermissionScope, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, scope, contract_id, task_id, plugin_id, tool, permissions, enabled, updated_at FROM plugin_permission_scopes ORDER BY updated_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PluginPermissionScope
	for rows.Next() {
		item, err := scanPluginPermissionScope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) PluginPermissionsForCall(contractID, taskID int64, pluginID, tool string) (PluginPermissionScope, bool, error) {
	rows, err := s.db.Query(`SELECT id, scope, contract_id, task_id, plugin_id, tool, permissions, enabled, updated_at
		FROM plugin_permission_scopes
		WHERE enabled = 1
			AND ((scope = 'task' AND task_id = ?) OR (scope = 'contract' AND contract_id = ?))
			AND (plugin_id = ? OR plugin_id = '*')
			AND (tool = ? OR tool = '*')
		ORDER BY
			CASE WHEN scope = 'task' THEN 0 ELSE 1 END,
			CASE WHEN plugin_id = ? THEN 0 ELSE 1 END,
			CASE WHEN tool = ? THEN 0 ELSE 1 END,
			updated_at DESC
		LIMIT 1`, taskID, contractID, pluginID, tool, pluginID, tool)
	if err != nil {
		return PluginPermissionScope{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return PluginPermissionScope{}, false, rows.Err()
	}
	item, err := scanPluginPermissionScope(rows)
	if err != nil {
		return PluginPermissionScope{}, false, err
	}
	return item, true, rows.Err()
}

func (s *Store) ListPluginCalls(limit int) ([]PluginCall, error) {
	if limit <= 0 {
		limit = 80
	}
	rows, err := s.db.Query(`SELECT id, plugin_id, tool, transport, contract_id, task_id, input, output, ok, duration, sandboxed, work_dir, error, created_at FROM plugin_calls ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []PluginCall
	for rows.Next() {
		var call PluginCall
		var okInt, sandboxedInt int
		if err := rows.Scan(&call.ID, &call.PluginID, &call.Tool, &call.Transport, &call.ContractID, &call.TaskID, &call.Input, &call.Output, &okInt, &call.Duration, &sandboxedInt, &call.WorkDir, &call.Error, &call.CreatedAt); err != nil {
			return nil, err
		}
		call.OK = okInt == 1
		call.Sandboxed = sandboxedInt == 1
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func scanPluginPermissionScope(scanner interface{ Scan(dest ...any) error }) (PluginPermissionScope, error) {
	var item PluginPermissionScope
	var enabled int
	if err := scanner.Scan(&item.ID, &item.Scope, &item.ContractID, &item.TaskID, &item.PluginID, &item.Tool, &item.Permissions, &enabled, &item.UpdatedAt); err != nil {
		return PluginPermissionScope{}, err
	}
	item.Enabled = enabled == 1
	return item, nil
}

func (s *Store) ListLogs(limit int) ([]LogEntry, error) {
	rows, err := s.db.Query(`SELECT id, level, scope, message, created_at FROM logs_tecnicos ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		if err := rows.Scan(&l.ID, &l.Level, &l.Scope, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *Store) UpsertKnowledge(topic string, rules []byte, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)
	_, err := s.db.Exec(`INSERT INTO knowledge_packs(topic, rules, format, expires_at) VALUES(?, ?, 'gob-rulemap', ?)
		ON CONFLICT(topic) DO UPDATE SET rules = excluded.rules, expires_at = excluded.expires_at, last_used_at = CURRENT_TIMESTAMP`,
		topic, rules, expiresAt)
	return err
}

func (s *Store) AddKnowledgeCandidate(contractID, taskID int64, domain, pattern, action, template, evidence string) (KnowledgeCandidate, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO knowledge_candidates(contract_id, task_id, domain, pattern, action, template, evidence, status)
		VALUES(?, ?, ?, ?, ?, ?, ?, 'pending')`, contractID, taskID, domain, pattern, action, template, evidence)
	if err != nil {
		return KnowledgeCandidate{}, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		return s.GetKnowledgeCandidateByTaskAction(taskID, action)
	}
	return s.GetKnowledgeCandidate(id)
}

func (s *Store) GetKnowledgeCandidate(id int64) (KnowledgeCandidate, error) {
	row := s.db.QueryRow(`SELECT id, contract_id, task_id, domain, pattern, action, template, evidence, status, created_at, approved_at
		FROM knowledge_candidates WHERE id = ?`, id)
	return scanKnowledgeCandidate(row)
}

func (s *Store) GetKnowledgeCandidateByTaskAction(taskID int64, action string) (KnowledgeCandidate, error) {
	row := s.db.QueryRow(`SELECT id, contract_id, task_id, domain, pattern, action, template, evidence, status, created_at, approved_at
		FROM knowledge_candidates WHERE task_id = ? AND action = ?`, taskID, action)
	return scanKnowledgeCandidate(row)
}

func (s *Store) ListKnowledgeCandidates(status string, limit int) ([]KnowledgeCandidate, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)
	query := `SELECT id, contract_id, task_id, domain, pattern, action, template, evidence, status, created_at, approved_at FROM knowledge_candidates`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeCandidate
	for rows.Next() {
		item, err := scanKnowledgeCandidate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SetKnowledgeCandidateStatus(id int64, status string) (KnowledgeCandidate, error) {
	var approvedAt any
	if status == "approved" {
		approvedAt = time.Now()
	}
	res, err := s.db.Exec(`UPDATE knowledge_candidates SET status = ?, approved_at = ? WHERE id = ? AND status = 'pending'`, status, approvedAt, id)
	if err != nil {
		return KnowledgeCandidate{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return KnowledgeCandidate{}, errors.New("regra candidata inexistente ou ja decidida")
	}
	return s.GetKnowledgeCandidate(id)
}

func (s *Store) GetKnowledge(topic string) (KnowledgeItem, error) {
	row := s.db.QueryRow(`SELECT id, topic, rules, format, expires_at, last_used_at, created_at FROM knowledge_packs WHERE topic = ? AND expires_at > CURRENT_TIMESTAMP`, topic)
	var item KnowledgeItem
	if err := row.Scan(&item.ID, &item.Topic, &item.Rules, &item.Format, &item.ExpiresAt, &item.LastUsedAt, &item.CreatedAt); err != nil {
		return KnowledgeItem{}, err
	}
	_, _ = s.db.Exec(`UPDATE knowledge_packs SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, item.ID)
	return item, nil
}

func (s *Store) ListKnowledge(limit int) ([]KnowledgeItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, topic, rules, format, expires_at, last_used_at, created_at FROM knowledge_packs WHERE expires_at > CURRENT_TIMESTAMP ORDER BY last_used_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []KnowledgeItem
	for rows.Next() {
		var item KnowledgeItem
		if err := rows.Scan(&item.ID, &item.Topic, &item.Rules, &item.Format, &item.ExpiresAt, &item.LastUsedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PurgeExpiredKnowledge() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM knowledge_packs WHERE expires_at <= CURRENT_TIMESTAMP OR last_used_at <= datetime('now', '-7 days')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) SystemBlocked() bool {
	var value string
	err := s.db.QueryRow(`SELECT value FROM sistema WHERE key = 'blocked'`).Scan(&value)
	return err == nil && value == "1"
}

func (s *Store) SetBlocked(blocked bool, reason string) error {
	value := "0"
	if blocked {
		value = "1"
	}
	_, err := s.db.Exec(`INSERT INTO sistema(key, value) VALUES('blocked', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, value)
	if err == nil && reason != "" {
		s.Log("WARN", "panic", reason)
	}
	return err
}

func (s *Store) dependenciesApproved(deps []int64) bool {
	for _, id := range deps {
		var status string
		if err := s.db.QueryRow(`SELECT status FROM tarefas WHERE id = ?`, id).Scan(&status); err != nil || status != StatusApproved {
			return false
		}
	}
	return true
}

func (s *Store) setTaskStatus(taskID int64, status string, readOnly bool) error {
	ro := 0
	if readOnly {
		ro = 1
	}
	res, err := s.db.Exec(`UPDATE tarefas SET status = ?, read_only = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND read_only = 0`, status, ro, taskID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("tarefa inexistente ou selada como read-only")
	}
	return nil
}

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (Task, error) {
	var t Task
	var depsRaw, payloadRaw string
	var readOnly int
	if err := scanner.Scan(&t.ID, &t.ContractID, &t.Title, &t.Role, &depsRaw, &t.Status, &t.Attempts, &readOnly, &payloadRaw, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Task{}, err
	}
	t.ReadOnly = readOnly == 1
	_ = json.Unmarshal([]byte(depsRaw), &t.Dependencies)
	_ = json.Unmarshal([]byte(payloadRaw), &t.Payload)
	if t.Payload == nil {
		t.Payload = map[string]any{}
	}
	return t, nil
}

func scanInterrogation(scanner taskScanner) (Interrogation, error) {
	var item Interrogation
	var questionsRaw, answersRaw string
	if err := scanner.Scan(&item.ID, &item.Input, &questionsRaw, &answersRaw, &item.NorthStar, &item.Constraints, &item.Deliverables, &item.Status, &item.ContractID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Interrogation{}, err
	}
	_ = json.Unmarshal([]byte(questionsRaw), &item.Questions)
	_ = json.Unmarshal([]byte(answersRaw), &item.Answers)
	if item.Questions == nil {
		item.Questions = []string{}
	}
	if item.Answers == nil {
		item.Answers = map[string]string{}
	}
	return item, nil
}

func scanFactoryBatch(scanner taskScanner) (FactoryBatch, error) {
	var item FactoryBatch
	var itemsRaw string
	if err := scanner.Scan(&item.ID, &item.BatchID, &item.Template, &itemsRaw, &item.Constraints, &item.Deliverables, &item.Status, &item.Total, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return FactoryBatch{}, err
	}
	_ = json.Unmarshal([]byte(itemsRaw), &item.Items)
	if item.Items == nil {
		item.Items = []string{}
	}
	return item, nil
}

func scanFactoryItem(scanner taskScanner) (FactoryItem, error) {
	var item FactoryItem
	var startedAt, finishedAt sql.NullTime
	if err := scanner.Scan(&item.ID, &item.BatchID, &item.Index, &item.Item, &item.ContractID, &item.Status, &startedAt, &finishedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return FactoryItem{}, err
	}
	if startedAt.Valid {
		item.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = finishedAt.Time
	}
	return item, nil
}

func scanHandoff(scanner taskScanner) (Handoff, error) {
	var item Handoff
	var payloadRaw string
	if err := scanner.Scan(&item.ID, &item.ContractID, &item.FromTaskID, &item.ToTaskID, &item.Kind, &payloadRaw, &item.SHA256, &item.CreatedAt); err != nil {
		return Handoff{}, err
	}
	_ = json.Unmarshal([]byte(payloadRaw), &item.Payload)
	if item.Payload == nil {
		item.Payload = map[string]any{}
	}
	return item, nil
}

func scanKnowledgeCandidate(scanner taskScanner) (KnowledgeCandidate, error) {
	var item KnowledgeCandidate
	var approvedAt sql.NullTime
	if err := scanner.Scan(&item.ID, &item.ContractID, &item.TaskID, &item.Domain, &item.Pattern, &item.Action, &item.Template, &item.Evidence, &item.Status, &item.CreatedAt, &approvedAt); err != nil {
		return KnowledgeCandidate{}, err
	}
	if approvedAt.Valid {
		item.ApprovedAt = approvedAt.Time
	}
	return item, nil
}
