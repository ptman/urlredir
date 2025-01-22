// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"flag"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func checkErr(tb testing.TB, err error) {
	tb.Helper()

	if err != nil {
		tb.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	flag.Parse()

	readConfigFile("config.json", &conf)

	if !testing.Short() {
		var err error

		pool, err = newPostgresDB()
		if err != nil {
			panic(err)
		}
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr,
		&slog.HandlerOptions{ //nolint:exhaustruct
			AddSource: true,
			Level:     slog.LevelDebug,
		})))

	code := m.Run()

	os.Exit(code)
}

func TestConfig(t *testing.T) {
	t.Parallel()

	conf := &config{} //nolint:exhaustruct
	readConfig(strings.NewReader("{}"), conf)

	js := conf.String()
	if js != `{"Listen":"","DB":"","Debug":false,"RealIPHeader":"","RemoteUserHeader":""}` {
		t.Error("Config: ", js)
	}
}

func TestConfigFromFile(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	conf := &config{} //nolint:exhaustruct

	readConfigFile("config.json.sample", conf)
}

func TestSetupServeMux(t *testing.T) {
	t.Parallel()

	db, err := newPostgresDB()
	checkErr(t, err)

	mux, ok := setupServeMux(db).(*http.ServeMux)
	if !ok {
		t.Fatal("failed type assertion")
	}

	_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/foo",
		nil))

	if pattern != "GET /{name}" {
		t.Error("Wrong pattern:", pattern)
	}

	_, pattern = mux.Handler(httptest.NewRequest(http.MethodGet, "/_admin",
		nil))

	if pattern != "GET /_admin" {
		t.Error("Wrong pattern:", pattern)
	}
}
