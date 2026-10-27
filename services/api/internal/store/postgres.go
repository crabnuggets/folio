package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/domain"
	"github.com/crabnuggets/folio/services/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, unavailable()
	}
	config.MaxConns = 8
	config.MinConns = 0
	config.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, unavailable()
	}
	p := &Postgres{pool}
	if err = p.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		pool.Close()
		return nil, unavailable()
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(760428192451)"); err == nil {
		var exists bool
		err = tx.QueryRow(ctx, "SELECT to_regclass('folio.schema_migrations') IS NOT NULL").Scan(&exists)
		var version int64
		if err == nil && exists {
			err = tx.QueryRow(ctx, "SELECT COALESCE(max(version),0) FROM folio.schema_migrations").Scan(&version)
		}
		if err == nil {
			for _, migration := range migrations.All {
				if migration.Version > version {
					if _, err = tx.Exec(ctx, migration.SQL); err != nil {
						break
					}
				}
			}
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		pool.Close()
		return nil, unavailable()
	}
	return p, nil
}
func (p *Postgres) begin(ctx context.Context, owner string) (pgx.Tx, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, unavailable()
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('folio.owner_id',$1,true)", owner); err != nil {
		tx.Rollback(context.Background())
		return nil, unavailable()
	}
	return tx, nil
}
func readState(ctx context.Context, tx pgx.Tx, owner string) (*domain.State, error) {
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM folio.owner_state WHERE owner_id=$1", owner).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewState(), nil
	}
	if err != nil {
		return nil, unavailable()
	}
	state := domain.NewState()
	if json.Unmarshal(raw, state) != nil {
		return nil, unavailable()
	}
	return state, nil
}
func (p *Postgres) Read(ctx context.Context, owner string) (*domain.State, error) {
	tx, err := p.begin(ctx, owner)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	return readState(ctx, tx, owner)
}
func (p *Postgres) Execute(ctx context.Context, owner, key, hash string, fn func(*domain.State) (Result, error)) (Result, bool, error) {
	if !domain.ValidID(key) {
		return Result{}, false, domain.Fail(400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a UUID")
	}
	tx, err := p.begin(ctx, owner)
	if err != nil {
		return Result{}, false, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))", "folio-owner:"+owner); err != nil {
		return Result{}, false, unavailable()
	}
	var oldHash string
	var oldResponse []byte
	err = tx.QueryRow(ctx, "SELECT request_hash,response FROM folio.idempotency_records WHERE owner_id=$1 AND key=$2", owner, key).Scan(&oldHash, &oldResponse)
	if err == nil {
		if hash != oldHash {
			return Result{}, false, domain.Fail(409, "IDEMPOTENCY_CONFLICT", "Idempotency key was already used for another request")
		}
		var result Result
		if json.Unmarshal(oldResponse, &result) != nil {
			return Result{}, false, unavailable()
		}
		return result, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, unavailable()
	}
	state, err := readState(ctx, tx, owner)
	if err != nil {
		return Result{}, false, err
	}
	result, err := fn(state)
	if err != nil {
		return Result{}, false, err
	}
	document, err := json.Marshal(state)
	if err != nil {
		return Result{}, false, unavailable()
	}
	response, err := json.Marshal(result)
	if err != nil {
		return Result{}, false, unavailable()
	}
	_, err = tx.Exec(ctx, `INSERT INTO folio.owner_state(owner_id,document) VALUES($1,$2) ON CONFLICT(owner_id) DO UPDATE SET document=excluded.document, revision=folio.owner_state.revision+1, updated_at=now()`, owner, document)
	if err != nil {
		return Result{}, false, unavailable()
	}
	_, err = tx.Exec(ctx, "INSERT INTO folio.idempotency_records(owner_id,key,request_hash,response) VALUES($1,$2,$3,$4)", owner, key, hash, string(response))
	if err != nil {
		return Result{}, false, unavailable()
	}
	if tx.Commit(ctx) != nil {
		return Result{}, false, unavailable()
	}
	return result, false, nil
}
func (p *Postgres) Ping(ctx context.Context) error {
	if p.pool.Ping(ctx) != nil {
		return unavailable()
	}
	return nil
}
func (p *Postgres) Close() { p.pool.Close() }
