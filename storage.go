// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
)

// Db wraps a database provider for us to enable testing
type Db interface {
	// begin transaction
	begin() (Tx, error)
	beginTx(context.Context) (Tx, error)
}

// Tx wraps a database transaction for us to enable testing
type Tx interface {
	// commit transaction
	commit() error
	// rollback transaction
	rollback() error
	// getURLnID(name) -> (url, id, error)
	getURLnID(string) (string, int64, error)
	// getIDnUser(name) -> (id, user, error)
	getIDnUser(name string) (int64, string, error)
	// addHit(url_id, ip, agent, referrer) -> error
	addHit(int64, net.IP, string, *string) error
	// addURL(name, url, user) -> error
	addURL(string, string, string) error
	// removeURL(name) -> error
	removeURL(string) error
	// urlsForUser(user) -> [{name,url,hits}], error
	urlsForUser(string) ([]map[string]string, error)
}

// postgresDb is a PostgreSQL specific implementation of Db
type postgresDb struct {
	db *sql.DB
}

// postgresTx is a PostgreSQL specific implementation of Tx
type postgresTx struct {
	tx *sql.Tx
}

// newPostgresDb returns an initialized postgresDb
func newPostgresDb() (Db, error) {
	const createTables = `
CREATE TABLE IF NOT EXISTS urls (
	id serial PRIMARY KEY,
	created timestamp with time zone NOT NULL DEFAULT now(),
	name text NOT NULL UNIQUE,
	url text NOT NULL,
	"user" text NOT NULL,
	hits int NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS hits (
	created timestamp with time zone NOT NULL DEFAULT now(),
	remotehost inet,
	referrer text,
	agent text,
	url_id int NOT NULL REFERENCES urls(id) ON DELETE CASCADE
)
`
	db, err := sql.Open("postgres", conf.DB.ConnInfo)
	if err != nil {
		return nil, err
	}
	if conf.DB.Schema != "" {
		if _, err = db.Exec(fmt.Sprintf(
			"CREATE SCHEMA IF NOT EXISTS %s",
			conf.DB.Schema)); err != nil {
			return nil, err
		}
		if _, err = db.Exec(fmt.Sprintf("SET search_path TO %s",
			conf.DB.Schema)); err != nil {
			return nil, err
		}
	}
	if _, err = db.Exec(createTables); err != nil {
		return nil, err
	}

	return &postgresDb{db}, nil
}

// begin begins a transaction
func (db *postgresDb) begin() (Tx, error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	if conf.DB.Schema != "" {
		if _, err = tx.Exec(fmt.Sprintf("SET search_path TO %s",
			conf.DB.Schema)); err != nil {
			return nil, err
		}
	}
	return &postgresTx{tx}, nil
}

// beginTx begins a cancellable transaction
func (db *postgresDb) beginTx(ctx context.Context) (Tx, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if conf.DB.Schema != "" {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"SET search_path TO %s", conf.DB.Schema)); err != nil {
			return nil, err
		}
	}
	return &postgresTx{tx}, nil
}

// commit the transaction
func (tx *postgresTx) commit() error {
	return tx.tx.Commit()
}

// rollback the transaction
func (tx *postgresTx) rollback() error {
	return tx.tx.Rollback()
}

// getURLnID returns URL and its ID
func (tx *postgresTx) getURLnID(name string) (string, int64, error) {
	const q = `UPDATE urls SET hits=hits+1 WHERE name=$1 RETURNING id,url`

	var id int64
	var url string
	err := tx.tx.QueryRow(q, name).Scan(&id, &url)
	return url, id, err
}

// getIDnUser returns the URL's ID and user
func (tx *postgresTx) getIDnUser(name string) (int64, string, error) {
	const q = `SELECT id,"user" FROM urls WHERE name=$1`

	var id int64
	var user string
	err := tx.tx.QueryRow(q, name).Scan(&id, &user)
	return id, user, err
}

// removeURL removes the URL speficied
func (tx *postgresTx) removeURL(name string) error {
	const q = `DELETE FROM urls WHERE name=$1`

	_, err := tx.tx.Exec(q, name)
	return err
}

// addHit adds a hit to the specific URL
func (tx *postgresTx) addHit(urlID int64, ip net.IP, agent string,
	referrer *string) error {
	const q = `
INSERT INTO hits (url_id, remotehost, agent, referrer)
VALUES ($1, $2, $3, $4)`

	_, err := tx.tx.Exec(q, urlID, ip.String(), agent, referrer)
	return err
}

// addURL adds a new URL to the database
func (tx *postgresTx) addURL(name, url, user string) error {
	const q = `INSERT INTO urls (name, url, "user") VALUES ($1, $2, $3)`

	_, err := tx.tx.Exec(q, name, url, user)
	return err
}

// urlsForUser returns all URLs for the given user
func (tx *postgresTx) urlsForUser(user string) ([]map[string]string, error) {
	const q = `SELECT name, url, hits FROM urls WHERE "user" = $1`

	rows, err := tx.tx.Query(q, user)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		if err = rows.Close(); err != nil {
			panic(err)
		}
	}(rows)

	urls := []map[string]string{}

	for rows.Next() {
		var name, url string
		var hits int
		if err = rows.Scan(&name, &url, &hits); err != nil {
			return nil, err
		}
		urls = append(urls, map[string]string{
			"name": name,
			"url":  url,
			"hits": strconv.Itoa(hits),
		})
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return urls, nil
}
