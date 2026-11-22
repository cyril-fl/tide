package postgrey

import (
	"context"
	"database/sql"
	"github.com/cyril-fl/tide/config"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	plumtransform "github.com/cyril-fl/plum/utils/transform"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type PostgresEnvironment struct {
	username string
	password string
	port     string
	host     string
	dbname   string
}

/*
NOTE HOST and PORT, are use as default values, but they could be
overwritten per build environment with LDFLAGS if omit in .env file
*/
const (
	HOST = "localhost"
	PORT = "5432"
)

func NewEnvironment() *PostgresEnvironment {
	return &PostgresEnvironment{}
}

func (env *PostgresEnvironment) ReadEnv() {
	env.username = strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	env.password = strings.TrimSpace(os.Getenv("POSTGRES_PASSWORD"))
	env.port = strings.TrimSpace(os.Getenv("PORT"))
	env.host = strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
	env.dbname = strings.TrimSpace(os.Getenv("POSTGRES_DB"))
}

// GetEnvURI escapes credentials via net/url; sslmode=disable since the local docker Postgres has no TLS.
func (env *PostgresEnvironment) GetEnvURI() string {
	uri := url.URL{
		Scheme:   "postgres",
		User:     env.getCredentials(),
		Host:     env.getHostAt(),
		Path:     env.GetEnvDBName(),
		RawQuery: "sslmode=disable",
	}
	return uri.String()
}

func (env *PostgresEnvironment) GetEnvDBName() string {
	if env.dbname == "" {
		env.dbname = config.Current.Name
	}

	return env.dbname
}

func (env *PostgresEnvironment) Connect() *bun.DB {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(env.GetEnvURI())))
	if testing.Testing() {
		lockTests.Do(func() { lockTests.conn = lockBase(sqldb) })
	}
	return bun.NewDB(sqldb, pgdialect.New())
}

// The test binaries of every go test running on the machine (agents, worktrees) share the _test base and the Silo
// bucket: one package at a time, or the schema, the purges and the sweeps of one break the others' tests.
var lockTests struct {
	sync.Once
	conn *sql.Conn
}

// lockBase waits for the lock of the base, held by a connection kept until the process exits, which releases it.
func lockBase(sqldb *sql.DB) *sql.Conn {
	ctx := context.Background()
	conn, err := sqldb.Conn(ctx)
	if err == nil {
		_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext('tide-tests'))")
	}
	if err != nil {
		panic("postgrey: lock the test base: " + err.Error())
	}
	return conn
}

func (env *PostgresEnvironment) getCredentials() *url.Userinfo {
	if env.username == "" || env.password == "" {
		return nil
	}
	return url.UserPassword(env.username, env.password)
}

func (env *PostgresEnvironment) getHostAt() string {
	if env.host == "" {
		env.host = HOST
	}
	if env.port == "" {
		env.port = PORT
	}

	return plumtransform.CleanJoin(":", env.host, env.port)
}
