// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseIP(t *testing.T) {
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
	fmt.Fprintf(w, r.RemoteAddr)
}

// testRequest takes care of some repetitive parts of testing.
func testRequest(t *testing.T, handler http.Handler, req *http.Request,
	code int,
) (*httptest.ResponseRecorder, string) {
	t.Helper()

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != code {
		t.Errorf("Status %d != %s %d", rr.Code, http.StatusText(code),
			code)
	}

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		panic(err)
	}

	return rr, strings.TrimSpace(string(body))
}

func TestRealIPMiddleware(t *testing.T) {
	handler := http.Handler(http.HandlerFunc(ipEchoHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "::1"

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "::1" {
		t.Error("RemoteAddr not ::1:", body)
	}

	handler = realIPMiddleware("X-Forwarded-For")(handler)

	req.Header.Set("X-Forwarded-For", httptest.DefaultRemoteAddr)

	_, body = testRequest(t, handler, req, http.StatusOK)

	if body != httptest.DefaultRemoteAddr {
		t.Error("RemoteAddr not", httptest.DefaultRemoteAddr, body)
	}
}

// helloHandler responds with a greeting to the user in context.
func helloHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user, ok := ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	fmt.Fprintf(w, "Hello, %s", user)
}

func TestPanicMiddleware(t *testing.T) {
	handler := panicMiddleware(http.HandlerFunc(ipEchoHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusOK)

	handler = panicMiddleware(http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusInternalServerError)

	if body != http.StatusText(http.StatusInternalServerError) {
		t.Error("No error", body)
	}
}

func TestStaticUserMiddleware(t *testing.T) {
	handler := panicMiddleware(http.HandlerFunc(helloHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	handler = staticUserMiddleware("test")(http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "Hello, test" {
		t.Error("Wrong result:", body)
	}
}

func TestRemoteUserMiddleware(t *testing.T) {
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

	_, db := initDB(t)

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

	_, db := initDB(t)

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

	if rr.Header().Get("Location") != cExampleCom {
		t.Error("Wrong location header:", rr.Header().Get("Location"))
	}

	if rr.Header().Get("Cache-Control") != "private, max-age=90" {
		t.Error("Wrong cache header:", rr.Header().Get("Cache-Control"))
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

	_, db := initDB(t)

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

func TestAdminHandler(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping db tests in short mode.")
	}

	_, db := initDB(t)

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

	if body != string(ErrInvalidURL) {
		t.Error("Wrong body", body)
	}

	// missing URL
	_, body = postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if body != string(ErrMissingURL) {
		t.Error("Wrong body", body)
	}

	// missing name
	_, body = postForm(t, mux, "/_admin", url.Values{
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if body != string(ErrMissingName) {
		t.Error("Wrong body", body)
	}

	// missing user
	_, body = postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
	}, http.StatusBadRequest)

	if body != string(ErrMissingUser) {
		t.Error("Wrong body", body)
	}

	// everything ok
	postForm(t, mux, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusSeeOther)
}
