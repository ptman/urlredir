// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"database/sql"
	"encoding/json"
	"expvar"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	_ "github.com/lib/pq"
)

// config is the (un)serializable config for urlredir.
type config struct {
	// Listen address, e.g. ":8080"
	Listen string
	// https://www.postgresql.org/docs/current/static/libpq-connect.html#LIBPQ-CONNSTRING
	DB string
	// Debug toggles exposing information over /debug/vars
	Debug bool
	// RealIPHeader is the name of the header where proxy supplies real IP
	RealIPHeader string
	// RemoteUserHeader it he name of the header where proxy supplies user
	RemoteUserHeader string
}

//nolint:gochecknoglobals
var (
	gitRev    string
	gitDirty  string
	revDate   time.Time
	goVersion string
	conf      config
	pool      *sql.DB
)

// String implements Stringer for expvar, returns JSON.
func (c config) String() string {
	b, err := json.Marshal(c) //nolint:musttag
	if err != nil {
		panic(err)
	}

	return string(b)
}

// readConfigFile reads config from file.
func readConfigFile(name string, conf *config) {
	cfile, err := os.Open(name)
	if err != nil {
		slog.Error("error opening config file", slog.Any("err", err))
		os.Exit(1)
	}

	readConfig(cfile, conf)
}

// readConfig reads config from io.Reader.
func readConfig(cfile io.Reader, conf *config) {
	var err error
	//nolint:musttag
	if err = json.NewDecoder(cfile).Decode(conf); err != nil {
		slog.Error("failed to decode config", slog.Any("err", err))
		os.Exit(1)
	}

	binfo, ok := debug.ReadBuildInfo()
	if ok {
		goVersion = binfo.GoVersion

		for _, s := range binfo.Settings {
			switch s.Key {
			case "vcs.revision":
				gitRev = s.Value
			case "vcs.time":
				t, err := time.Parse(time.RFC3339, s.Value)
				if err != nil {
					slog.Error("error parsing vcs.time",
						slog.String("time", s.Value),
						slog.Any("err", err))
					os.Exit(1)
				}

				revDate = t
			case "vcs.modified":
				gitDirty = s.Value
			}
		}
	}
}

// setupServeMux returns a set up http.Handler.
func setupServeMux(db *sql.DB) http.Handler {
	mux := http.NewServeMux()

	if conf.Debug {
		expvar.NewString("gitrev").Set(gitRev)
		expvar.NewString("gitDirty").Set(gitDirty)
		expvar.NewString("revdate").Set(revDate.Format(time.RFC3339))
		expvar.Publish("config", conf)

		mux.Handle("GET /debug/vars", expvar.Handler())
	}

	mws := chain{panicMiddleware, loggerMiddleware, dbMiddleware(db)}

	if conf.RealIPHeader != "" {
		mws = append(mws, realIPMiddleware(conf.RealIPHeader))
	}

	if conf.RemoteUserHeader != "" {
		mws = append(mws, remoteUserMiddleware(conf.RemoteUserHeader))
	} else {
		mws = append(mws, staticUserMiddleware("test"))
	}

	mux.Handle("GET /{name}", mws.applyE(redirHandler))
	mux.Handle("DELETE /{name}", mws.applyE(deleteHandler))
	mux.Handle("GET /_admin", mws.applyE(adminGetHandler))
	mux.Handle("POST /_admin", mws.applyE(adminPostHandler))

	return mux
}

// main should be kept small as it is hard to test.
func main() {
	logLevel := new(slog.LevelVar)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr,
		&slog.HandlerOptions{ //nolint:exhaustruct
			AddSource: true,
			Level:     logLevel,
		})))

	var err error

	readConfigFile("config.json", &conf)

	if conf.Debug {
		logLevel.Set(slog.LevelDebug)
	}

	pool, err = newPostgresDB()
	if err != nil {
		slog.Error("error opening database", slog.Any("err", err))
		os.Exit(1)
	}

	mux := setupServeMux(pool)

	slog.Info("Listening", slog.String("goversion", goVersion),
		slog.String("gitRev", gitRev), slog.Any("revDate", revDate),
		slog.String("gitDirty", gitDirty),
		slog.String("addr", conf.Listen))

	srv := &http.Server{ //nolint:exhaustruct
		Handler:           mux,
		ReadTimeout:       time.Minute,
		WriteTimeout:      time.Minute,
		ReadHeaderTimeout: time.Minute,
		IdleTimeout:       time.Minute,
		Addr:              conf.Listen,
	}

	if err := srv.ListenAndServe(); err != nil {
		slog.Error("error listening", slog.Any("err", err))
		os.Exit(1)
	}
}
