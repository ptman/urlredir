// Copyright (c) 2017-2020 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	conf := &config{}
	readConfig(strings.NewReader("{}"), conf)

	js := conf.String()
	if js != `{"Listen":"","DB":{"ConnInfo":"","Schema":""},"Debug":false,"RealIPHeader":"","RemoteUserHeader":""}` {
		t.Error("Config: ", js)
	}
}

func TestConfigFromFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	conf := &config{}

	readConfigFile("config.json.sample", conf)
}

func TestSetupServeMux(t *testing.T) {
	mux := setupServeMux(&fakedb{}).(*http.ServeMux)

	_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/foo",
		nil))

	if pattern != "/" {
		t.Error("Wrong pattern:", pattern)
	}

	_, pattern = mux.Handler(httptest.NewRequest(http.MethodGet, "/_admin",
		nil))

	if pattern != "/_admin" {
		t.Error("Wrong pattern:", pattern)
	}
}
