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

	configFile := "config.json"
	if _, err := os.Stat(configFile); err != nil {
		if os.IsNotExist(err) {
			configFile = "config.json.sample"
		} else {
			panic(err)
		}
	}

	readConfigFile(configFile, &conf)

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

func TestApplyEnvOverrides(t *testing.T) {
	t.Run("overrides when env vars are set", func(t *testing.T) {
		t.Setenv("PORT", "1234")
		t.Setenv("DATABASE_URL", "postgres://db.example/urlredir")

		c := &config{ //nolint:exhaustruct
			Listen: ":9999",
			DB:     "host=/run/postgresql dbname=urlredir",
		}

		applyEnvOverrides(c)

		if c.Listen != ":1234" {
			t.Fatalf("Listen = %q, want %q", c.Listen, ":1234")
		}

		if c.DB != "postgres://db.example/urlredir" {
			t.Fatalf("DB = %q, want %q", c.DB, "postgres://db.example/urlredir")
		}
	})

	t.Run("keeps config when env vars are unset", func(t *testing.T) {
		t.Setenv("PORT", "")
		t.Setenv("DATABASE_URL", "")

		c := &config{ //nolint:exhaustruct
			Listen: ":8080",
			DB:     "host=/run/postgresql dbname=urlredir",
		}

		applyEnvOverrides(c)

		if c.Listen != ":8080" {
			t.Fatalf("Listen = %q, want %q", c.Listen, ":8080")
		}

		if c.DB != "host=/run/postgresql dbname=urlredir" {
			t.Fatalf("DB = %q, want %q", c.DB, "host=/run/postgresql dbname=urlredir")
		}
	})
}

func TestSetupServeMux(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	db, err := newPostgresDB()
	checkErr(t, err)

	mux, ok := setupServeMux(db).(*http.ServeMux)
	if !ok {
		t.Fatal("failed type assertion")
	}

	_, pattern := mux.Handler(httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/foo", nil))

	if pattern != "GET /{name}" {
		t.Error("Wrong pattern:", pattern)
	}

	_, pattern = mux.Handler(httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/_admin", nil))

	if pattern != "GET /_admin" {
		t.Error("Wrong pattern:", pattern)
	}
}
