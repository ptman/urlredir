// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseIP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		ip string
		ok bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"foo", false},
	}

	for _, tc := range testCases {
		_, err := parseIP(tc.ip)
		if tc.ok && err != nil {
			t.Errorf("Error parsing IP: %v", err)
		} else if !tc.ok && err == nil {
			t.Errorf("No error on IP: %v", tc.ip)
		}
	}
}

// ipEchoHandler responds with the client IP address.
func ipEchoHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := fmt.Fprintf(w, "%s", r.RemoteAddr); err != nil {
		slog.Error("error", slog.Any("err", err))
	}
}

// testRequest takes care of some repetitive parts of testing.
func testRequest(t *testing.T, handler http.Handler, req *http.Request,
	code int,
) (*httptest.ResponseRecorder, string) {
	t.Helper()

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Code, code; got != want {
		t.Errorf("Status: got %s %d , want %s %d", http.StatusText(got),
			got, http.StatusText(want), want)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		panic(err)
	}

	return rr, strings.TrimSpace(string(body))
}

func TestRealIPMiddleware(t *testing.T) {
	t.Parallel()

	handler := http.Handler(http.HandlerFunc(ipEchoHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "::1"

	_, body := testRequest(t, handler, req, http.StatusOK)

	if got, want := body, "::1"; got != want {
		t.Errorf("remoteAddr: got %s , want %s", got, want)
	}

	handler = realIPMiddleware("X-Forwarded-For")(handler)

	req.Header.Set("X-Forwarded-For", httptest.DefaultRemoteAddr)

	_, body = testRequest(t, handler, req, http.StatusOK)

	if got, want := body, httptest.DefaultRemoteAddr; got != want {
		t.Errorf("RemoteAddr: got %s , want %s", got, want)
	}
}

// helloHandler responds with a greeting to the user in context.
func helloHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := must(getUser(ctx))

	if _, err := fmt.Fprintf(w, "Hello, %s", user); err != nil {
		slog.Error("error", slog.Any("err", err))
	}
}

func TestPanicMiddleware(t *testing.T) {
	t.Parallel()

	handler := panicMiddleware(http.HandlerFunc(ipEchoHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusOK)

	handler = panicMiddleware(http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusInternalServerError)

	if got, want := body,
		http.StatusText(http.StatusInternalServerError); got != want {
		t.Errorf("Missing error: got %s , want %s", got, want)
	}
}

func TestStaticUserMiddleware(t *testing.T) {
	t.Parallel()

	handler := panicMiddleware(http.HandlerFunc(helloHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	handler = staticUserMiddleware("test")(http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusOK)

	if got, want := body, "Hello, test"; got != want {
		t.Errorf("Wrong result: got %s , want %s", got, want)
	}
}

func TestRemoteUserMiddleware(t *testing.T) {
	t.Parallel()

	handler := remoteUserMiddleware("X-Remote-User")(
		http.HandlerFunc(helloHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	req.Header.Set("X-Remote-User", "foo")

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "Hello, foo" {
		t.Error("Wrong result:", body)
	}
}

func TestDBHandler(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	db := initDB(t)

	handler := panicMiddleware(dbMiddleware(db)(
		http.HandlerFunc(ipEchoHandler)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusOK)
}

func TestRedirHandler(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	// missing dbMiddleware
	handler := panicMiddleware(withError(redirHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	db := initDB(t)

	// missing URL
	handler = panicMiddleware(dbMiddleware(db)(withError(redirHandler)))
	req = httptest.NewRequest(http.MethodGet, "/foo", nil)

	testRequest(t, handler, req, http.StatusNotFound)

	// everything ok
	mux := http.NewServeMux()
	mux.Handle("GET /{name}", chain{panicMiddleware, dbMiddleware(db)}.
		applyE(redirHandler))

	req = httptest.NewRequest(http.MethodGet, "/foo", nil)

	req.Header.Set("Referer", "http://example.org")

	rr, _ := testRequest(t, mux, req, http.StatusMovedPermanently)

	if got, want := rr.Header().Get("Location"), cExampleCom; got != want {
		t.Errorf("Wrong location header: got %s , want %s", got, want)
	}

	if got, want := rr.Header().Get("Cache-Control"),
		"private, max-age=90"; got != want {
		t.Errorf("Wrong cache header: got %s , want %s", got, want)
	}
}

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	// missing dbMiddleware
	handler := panicMiddleware(withError(redirHandler))
	req := httptest.NewRequest(http.MethodDelete, "/foo", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	db := initDB(t)

	// missing user
	handler = panicMiddleware(dbMiddleware(db)(withError(deleteHandler)))

	testRequest(t, handler, req, http.StatusInternalServerError)

	// empty user
	handler = panicMiddleware(remoteUserMiddleware("X-Remote-User")(
		dbMiddleware(db)(withError(deleteHandler))))

	testRequest(t, handler, req, http.StatusBadRequest)

	// wrong user
	mux := http.NewServeMux()
	mux.Handle("DELETE /{name}", chain{
		panicMiddleware,
		staticUserMiddleware("bar"), dbMiddleware(db),
	}.
		applyE(deleteHandler))

	testRequest(t, mux, req, http.StatusForbidden)

	// everythink ok
	mux = http.NewServeMux()
	mux.Handle("DELETE /{name}", chain{
		panicMiddleware,
		staticUserMiddleware("test"), dbMiddleware(db),
	}.
		applyE(deleteHandler))

	testRequest(t, mux, req, http.StatusOK)
}

// postForm is a test helper for POST requests.
//
//nolint:unparam
func postForm(t *testing.T, handler http.Handler, target string,
	values url.Values, code int,
) (*httptest.ResponseRecorder, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, target,
		strings.NewReader(values.Encode()))

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return testRequest(t, handler, req, code)
}

func TestAdminHandler(t *testing.T) { //nolint:funlen
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	db := initDB(t)

	mux := http.NewServeMux()
	mws := chain{
		panicMiddleware, staticUserMiddleware("test"),
		dbMiddleware(db),
	}
	mux.Handle("GET /", mws.applyE(adminGetHandler))
	mux.Handle("POST /", mws.applyE(adminPostHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// ok GET request
	testRequest(t, mux, req, http.StatusOK)

	// missing POST form
	req = httptest.NewRequest(http.MethodPost, "/_admin", nil)

	testRequest(t, mux, req, http.StatusBadRequest)

	// bad URL
	_, body := postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {":foo/bar"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if got, want := body, string(ErrInvalidURL); got != want {
		t.Errorf("Wrong body: got %s , want %s", got, want)
	}

	// missing URL
	_, body = postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if got, want := body, string(ErrMissingURL); got != want {
		t.Errorf("Wrong body: got %s , want %s", got, want)
	}

	// missing name
	_, body = postForm(t, mux, "/_admin", url.Values{
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if got, want := body, string(ErrMissingName); got != want {
		t.Errorf("Wrong body: got %s , want %s", got, want)
	}

	// missing user
	_, body = postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
	}, http.StatusBadRequest)

	if got, want := body, string(ErrMissingUser); got != want {
		t.Errorf("Wrong body: got %s , want %s", got, want)
	}

	// everything ok
	postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusSeeOther)
}
