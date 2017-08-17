// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

import (
	"encoding/json"
	"expvar"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// config is the (un)serializable config for urlredir
type config struct {
	// Listen address, e.g. ":8080"
	Listen string
	DB     struct {
		// ConnInfo must be a libpq "conninfo" connection string
		// https://www.postgresql.org/docs/current/static/libpq-connect.html#LIBPQ-CONNSTRING
		ConnInfo string
		// Schema is the optional PostgreSQL schema to use
		Schema string
	}
	// RealIPHeader is the name of the header where proxy supplies real IP
	RealIPHeader string
	// RemoteUserHeader it he name of the header where proxy supplies user
	RemoteUserHeader string
}

var (
	// gitRev is set by the build process to the revision being built
	gitRev string
	// revDateS is set by the build process to the revision timestamp
	revDateS = "0001-01-01T00:00:00+00:00"
	// revDate is parsed from RevDateS
	revDate time.Time
	conf    config
	db      Db
)

// String() produces JSON for expvar
func (c config) String() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// readConfigFile reads config from file
func readConfigFile(name string, conf *config) {
	cfile, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	readConfig(cfile, conf)
}

// readConfig reads config from io.Reader
func readConfig(cfile io.Reader, conf *config) {
	var err error
	if err = json.NewDecoder(cfile).Decode(conf); err != nil {
		log.Fatal(err)
	}
	revDate, err = time.Parse(time.RFC3339, revDateS)
	if err != nil {
		log.Fatal(err)
	}
}

// setupServeMux returns a set up http.Handler
func setupServeMux(db Db) http.Handler {
	mux := http.DefaultServeMux

	expvar.NewString("gitrev").Set(gitRev)
	expvar.NewString("revdate").Set(revDate.Format(time.RFC3339))
	expvar.Publish("config", conf)
	/* go1.8
	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	*/

	handler := dbHandler(db, http.HandlerFunc(indexHandler))

	if conf.RealIPHeader != "" {
		handler = realIPHandler(conf.RealIPHeader, handler)
	}

	admin := dbHandler(db, http.HandlerFunc(adminHandler))

	if conf.RemoteUserHeader != "" {
		handler = remoteUserHandler(conf.RemoteUserHeader, handler)
		admin = remoteUserHandler(conf.RemoteUserHeader, admin)
	} else {
		handler = staticUserHandler("test", handler)
		admin = staticUserHandler("test", admin)
	}

	mux.Handle("/", panicHandler(handler))
	mux.Handle("/_admin", panicHandler(admin))
	return mux
}

// main() should be kept small as it is hard to test
func main() {
	var err error
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	readConfigFile("config.json", &conf)
	db, err = newPostgresDb()
	if err != nil {
		log.Fatal(err)
	}

	_ = setupServeMux(db)

	log.Print(gitRev)
	log.Print(revDate.Format(time.RFC3339))
	log.Println("Listening on", conf.Listen)
	log.Fatal(http.ListenAndServe(conf.Listen, nil))
}
