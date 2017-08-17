// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"
)

// testSetup creates a temporary schema for testing
func testSetup() {
	readConfigFile("config.json", &conf)
	conf.DB.Schema = "urlredirtest" + strconv.Itoa(rand.Intn(1000))
	var err error
	db, err = newPostgresDb()
	if err != nil {
		panic(err)
	}
}

// testTeardown drops the temporary schema user for testing
func testTeardown() {
	_, err := db.(*postgresDb).db.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE",
		conf.DB.Schema))
	if err != nil {
		panic(err)
	}
}

// TestMain runs setup and teardown when not running short tests
func TestMain(m *testing.M) {
	flag.Parse()

	if !testing.Short() {
		testSetup()
	}

	code := m.Run()

	if !testing.Short() {
		testTeardown()
	}

	os.Exit(code)
}

func TestNewPostgresDb(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	d, err := newPostgresDb()
	if err != nil {
		t.Fatal("Error initializing db:", err)
	}
	tx, err := d.begin()
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}
	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	err = tx.commit()
	if err != nil {
		t.Fatal("Error committing transaction:", err)
	}
}

func TestAddURL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestGetURLnID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	// add something to get
	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	url, _, err := tx.getURLnID("foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	if url != "http://example.com" {
		t.Error("Got wrong URL:", url)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestGetIDnUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	// add something to get
	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	_, user, err := tx.getIDnUser("foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	if user != "test" {
		t.Error("Got wrong user:", user)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestRemoveURL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	// add something to remove
	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	_, _, err = tx.getURLnID("foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	err = tx.removeURL("foo")
	if err != nil {
		t.Fatal("Error removing URL:", err)
	}

	_, _, err = tx.getURLnID("foo")
	if err != sql.ErrNoRows {
		t.Error("Error, should not find URL:", err)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestAddHit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	// add something that can be hit
	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	_, id, err := tx.getURLnID("foo")
	if err != nil {
		t.Fatal("Error getting URL:", err)
	}

	referrer := "http://example.org"
	err = tx.addHit(id, net.IPv4(127, 0, 0, 1), "testagent", &referrer)
	if err != nil {
		t.Fatal("Error adding hit:", err)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}

func TestURLsForUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	tx, err := db.beginTx(context.Background())
	if err != nil {
		t.Fatal("Error beginning transaction:", err)
	}

	// add something to retrieve
	err = tx.addURL("foo", "http://example.com", "test")
	if err != nil {
		t.Fatal("Error adding URL:", err)
	}

	urls, err := tx.urlsForUser("test")
	if err != nil {
		t.Fatal("Error getting URLs:", err)
	}

	if len(urls) != 1 {
		t.Error("Got wrong number of URLs:", len(urls))
	}

	if urls[0]["name"] != "foo" {
		t.Error("Got wrong URLs:", urls)
	}

	if urls[0]["url"] != "http://example.com" {
		t.Error("Got wrong URLs:", urls)
	}

	if urls[0]["hits"] != "0" {
		t.Error("Got wrong URLs:", urls)
	}

	err = tx.rollback()
	if err != nil {
		t.Fatal("Error rolling back transaction:", err)
	}
}
