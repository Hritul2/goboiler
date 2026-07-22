package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/utils/config"
)

func TestMongo_DriverName(t *testing.T) {
	require.Equal(t, "mongodb", (&Mongo{}).DriverName())
}

func TestClientOptions(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.DbConfig
		wantMaxPool     uint64
		wantMinPool     uint64
		wantMaxIdleTime time.Duration
	}{
		{
			name: "config_json_defaults",
			cfg: config.DbConfig{
				Uri: "mongodb://localhost:27017", Name: "t",
				MaxOpenConns: 25, MaxIdleConns: 10,
				ConnMaxLifetime: 300, ConnMaxIdleTime: 60,
			},
			wantMaxPool: 25, wantMinPool: 10, wantMaxIdleTime: 60 * time.Second,
		},
		{
			name: "minimal_pool",
			cfg: config.DbConfig{
				Uri: "mongodb://db.example.com:27018", Name: "t",
				MaxOpenConns: 1, MaxIdleConns: 1,
				ConnMaxLifetime: 1, ConnMaxIdleTime: 1,
			},
			wantMaxPool: 1, wantMinPool: 1, wantMaxIdleTime: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := clientOptions(&tt.cfg)

			require.NotNil(t, opts.MaxPoolSize)
			require.Equal(t, tt.wantMaxPool, *opts.MaxPoolSize)
			require.NotNil(t, opts.MinPoolSize)
			require.Equal(t, tt.wantMinPool, *opts.MinPoolSize)
			require.NotNil(t, opts.MaxConnIdleTime)
			require.Equal(t, tt.wantMaxIdleTime, *opts.MaxConnIdleTime)
			require.NotNil(t, opts.Monitor, "otelmongo command monitor must be attached")
			require.NotEmpty(t, opts.Hosts, "ApplyURI must parse hosts")
		})
	}
}
