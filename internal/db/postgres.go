package db

// Postgres implementation — commented out until pgx is adopted.
//
// To enable:
//
//	go get github.com/jackc/pgx/v5
//	go get github.com/exaring/otelpgx   // pgx v5 OTel tracer
//
// then uncomment below, add the imports, and swap the active store in
// db.go's Init (see the marked block there). DbConfig maps directly:
//
//	Uri             → pgxpool.ParseConfig
//	MaxOpenConns    → pcfg.MaxConns
//	MaxIdleConns    → pcfg.MinConns
//	ConnMaxLifetime → pcfg.MaxConnLifetime (seconds)
//	ConnMaxIdleTime → pcfg.MaxConnIdleTime (seconds)
//
// Driver-error mapping (pgerrcode → sentinels) belongs in errors.go next to
// MapDBError; see the commented table in internal/errors/db.go.
//
// type Postgres struct {
// 	pool *pgxpool.Pool
// 	log  zerolog.Logger
// }
//
// var _ Database = (*Postgres)(nil)
//
// func newPostgres(ctx context.Context, cfg *config.DbConfig, log zerolog.Logger) (*Postgres, error) {
// 	pcfg, err := pgxpool.ParseConfig(cfg.Uri)
// 	if err != nil {
// 		return nil, err
// 	}
// 	pcfg.MaxConns = int32(cfg.MaxOpenConns)
// 	pcfg.MinConns = int32(cfg.MaxIdleConns)
// 	pcfg.MaxConnLifetime = time.Duration(cfg.ConnMaxLifetime) * time.Second
// 	pcfg.MaxConnIdleTime = time.Duration(cfg.ConnMaxIdleTime) * time.Second
// 	pcfg.ConnConfig.Tracer = otelpgx.NewTracer()
//
// 	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	p := &Postgres{pool: pool, log: log}
//
// 	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
// 	defer cancel()
// 	if err := p.Ping(pingCtx); err != nil {
// 		pool.Close()
// 		return nil, err
// 	}
//
// 	log.Info().Msg("postgres connected")
// 	return p, nil
// }
//
// func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
// func (p *Postgres) Close(_ context.Context) error  { p.pool.Close(); return nil }
// func (p *Postgres) DriverName() string             { return "postgres" }
//
// // Pool exposes the raw pgx pool for repositories.
// func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }
