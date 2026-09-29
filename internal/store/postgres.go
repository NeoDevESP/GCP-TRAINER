package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema is applied at startup (idempotent). Documents are JSONB with GIN
// indexes; analytics can later be exported to BigQuery.
const Schema = `
CREATE TABLE IF NOT EXISTS documents (
  coll       text        NOT NULL,
  id         text        NOT NULL,
  data       jsonb       NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (coll, id)
);
CREATE INDEX IF NOT EXISTS documents_data_gin ON documents USING gin (data jsonb_path_ops);
-- Supabase publishes the public schema through its REST API: row-level
-- security with no policies closes it. The platform connects as the table
-- owner, which RLS does not restrict.
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
`

// Postgres stores documents in PostgreSQL / Cloud SQL / AlloyDB.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres connects and migrates.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// Transaction-mode poolers (PgBouncer, Supabase on port 6543) do not keep
	// prepared statements between transactions.
	if cfg.ConnConfig.Port == 6543 && !strings.Contains(dsn, "default_query_exec_mode") {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
	if !strings.Contains(dsn, "pool_max_conns") {
		cfg.MaxConns = 5 // hosted free tiers limit connections
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(c, Schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Put(coll, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(context.Background(), `INSERT INTO documents (coll,id,data,updated_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (coll,id) DO UPDATE SET data=EXCLUDED.data, updated_at=now()`, coll, id, string(b)) // text works with the extended and the simple protocol
	return err
}

func (p *Postgres) Get(coll, id string, v any) error {
	var b []byte
	err := p.pool.QueryRow(context.Background(), `SELECT data FROM documents WHERE coll=$1 AND id=$2`, coll, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (p *Postgres) Delete(coll, id string) error {
	_, err := p.pool.Exec(context.Background(), `DELETE FROM documents WHERE coll=$1 AND id=$2`, coll, id)
	return err
}

func (p *Postgres) List(coll string) ([]json.RawMessage, error) {
	rows, err := p.pool.Query(context.Background(), `SELECT data FROM documents WHERE coll=$1 ORDER BY id`, coll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}

func (p *Postgres) Close() error {
	p.pool.Close()
	return nil
}
