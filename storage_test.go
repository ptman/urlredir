// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"testing"
)

const cExampleCom = "http://example.com"

func initDB(tb testing.TB) (context.Context, *sql.Conn) {
	tb.Helper()

	ctx := context.Background()

	//nolint:gosec
	schemaName := fmt.Sprintf("%s_%d", tb.Name(), rand.Intn(1000))

	conn, err := pool.Conn(ctx)
	checkErr(tb, err)

	//nolint:perfsprint
	_, err = conn.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s",
		schemaName))
	checkErr(tb, err)

	_, err = conn.ExecContext(ctx,
		"SELECT pg_catalog.set_config('search_path', $1, false)",
		schemaName)
	checkErr(tb, err)

	checkErr(tb, ensureSchema(conn))

	_, err = conn.ExecContext(ctx,
		`INSERT INTO urls (name, url, "user") values ($1, $2, $3)`,
		"foo", cExampleCom, "test")
	checkErr(tb, err)

	tb.Cleanup(func() {
		_, err := conn.ExecContext(ctx, fmt.Sprintf(
			"DROP SCHEMA %s CASCADE", schemaName))
		checkErr(tb, err)
	})

	return ctx, conn
}

func initTx(ctx context.Context, tb testing.TB, conn *sql.Conn) *sql.Tx {
	tb.Helper()

	tx, err := conn.BeginTx(ctx, nil)
	checkErr(tb, err)

	tb.Cleanup(func() {
		err := tx.Commit()
		checkErr(tb, err)
	})

	return tx
}

func TestNewPostgresDB(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	_, err := newPostgresDB()
	if err != nil {
		t.Fatal("Error initializing db:", err)
	}
}

func TestAddURL(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	err := addURL(ctx, tx, "bar", cExampleCom, "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}
}

func TestGetURLnID(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	url, _, err := getURLnID(ctx, tx, "foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	if url != cExampleCom {
		t.Error("Got wrong URL:", url)
	}
}

func TestGetIDnUser(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	_, user, err := getIDnUser(ctx, tx, "foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	if user != "test" {
		t.Error("Got wrong user:", user)
	}
}

func TestRemoveURL(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	_, _, err := getURLnID(ctx, tx, "foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	if err := removeURL(ctx, tx, "foo"); err != nil {
		t.Fatal("Error removing URL:", err)
	}

	if _, _, err := getURLnID(ctx, tx, "foo"); !errors.Is(err,
		sql.ErrNoRows) {
		t.Error("Error, should not find URL:", err)
	}
}

func TestAddHit(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	_, id, err := getURLnID(ctx, tx, "foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	referrer := cExampleCom

	if err := addHit(ctx, tx, id, net.IPv4(127, 0, 0, 1), "testagent",
		&referrer); err != nil {
		t.Fatal("Error adding hit:", err)
	}
}

func TestURLsForUser(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	ctx, db := initDB(t)
	tx := initTx(ctx, t, db)

	urls, err := urlsForUser(ctx, tx, "test")
	if err != nil {
		t.Fatal("Error getting URLs:", err)
	}

	if len(urls) != 1 {
		t.Error("Got wrong number of URLs:", len(urls))
	}

	if urls[0]["name"] != "foo" {
		t.Error("Got wrong URLs:", urls)
	}

	if urls[0]["url"] != cExampleCom {
		t.Error("Got wrong URLs:", urls)
	}

	if urls[0]["hits"] != "0" {
		t.Error("Got wrong URLs:", urls)
	}
}
