// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
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

// ipEchoHandler responds with the client IP address
func ipEchoHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, r.RemoteAddr)
}

// testRequest takes care of some repetitive parts of testing
func testRequest(t *testing.T, handler http.Handler, req *http.Request,
	code int) (*httptest.ResponseRecorder, string) {
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != code {
		t.Errorf("Status %d != %s %d", rr.Code, http.StatusText(code),
			code)
	}

	body, err := ioutil.ReadAll(rr.Body)
	if err != nil {
		panic(err)
	}

	return rr, strings.TrimSpace(string(body))
}

func TestRealIPHandler(t *testing.T) {
	handler := http.HandlerFunc(ipEchoHandler)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "::1"

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "::1" {
		t.Error("RemoteAddr not ::1:", body)
	}

	handler = realIPHandler("X-Forwarded-For", handler)
	req.Header.Set("X-Forwarded-For", httptest.DefaultRemoteAddr)

	_, body = testRequest(t, handler, req, http.StatusOK)

	if body != httptest.DefaultRemoteAddr {
		t.Error("RemoteAddr not", httptest.DefaultRemoteAddr, body)
	}
}

// helloHandler responds with a greeting to the user in context
func helloHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	fmt.Fprintf(w, "Hello, %s", user)
}

func TestPanicHandler(t *testing.T) {
	handler := panicHandler(http.HandlerFunc(ipEchoHandler))
	req := httptest.NewRequest("GET", "/", nil)

	testRequest(t, handler, req, http.StatusOK)

	handler = panicHandler(http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusInternalServerError)

	if body != http.StatusText(http.StatusInternalServerError) {
		t.Error("No error", body)
	}
}

func TestStaticUserHandler(t *testing.T) {
	handler := panicHandler(http.HandlerFunc(helloHandler))
	req := httptest.NewRequest("GET", "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	handler = staticUserHandler("test", http.HandlerFunc(helloHandler))

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "Hello, test" {
		t.Error("Wrong result:", body)
	}
}

func TestRemoteUserHandler(t *testing.T) {
	handler := remoteUserHandler("X-Remote-User",
		http.HandlerFunc(helloHandler))
	req := httptest.NewRequest("GET", "/", nil)

	req.Header.Set("X-Remote-User", "foo")

	_, body := testRequest(t, handler, req, http.StatusOK)

	if body != "Hello, foo" {
		t.Error("Wrong result:", body)
	}
}

// realerrdb mocks a very problematic db connection
type realerrdb struct{}

func (*realerrdb) begin() (Tx, error) {
	return nil, errors.New("No tx for you")
}

// errdb mocks a problematic db connection
type errdb struct{}

func (*errdb) begin() (Tx, error) {
	return &errtx{}, nil
}

type errtx struct{}

func (*errtx) commit() error {
	return errors.New("Failed to commit")
}

func (*errtx) rollback() error {
	return errors.New("Failed to rollback")
}

func (*errtx) addHit(id int64, ip net.IP, agent string, refer *string) error {
	return errors.New("Failed to add hit")
}

func (*errtx) addURL(name, url, user string) error {
	return errors.New("Failed to add URL")
}

func (*errtx) getURLnID(name string) (string, int64, error) {
	return "", 0, errors.New("Failed to get URL")
}

func (*errtx) getIDnUser(name string) (int64, string, error) {
	return 0, "", errors.New("Failed to get URL")
}

func (*errtx) removeURL(name string) error {
	return errors.New("Failed to remove URL")
}

func (*errtx) urlsForUser(user string) ([]map[string]string, error) {
	return nil, errors.New("Failed to get URLs")
}

func TestDbHandler(t *testing.T) {
	handler := panicHandler(dbHandler(&realerrdb{},
		http.HandlerFunc(ipEchoHandler)))
	req := httptest.NewRequest("GET", "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	handler = panicHandler(dbHandler(&errdb{},
		http.HandlerFunc(ipEchoHandler)))

	// This returns 200 OK, but has an ISE since the error occurs after
	// ipEchoHandler in dbHandler when trying to commit
	_, body := testRequest(t, handler, req, http.StatusOK)

	if !strings.HasSuffix(body,
		http.StatusText(http.StatusInternalServerError)) {
		t.Error("No error", body)
	}
}

// notfounddb mocks a database that doesn't return any results
type notfounddb struct{}

func (*notfounddb) begin() (Tx, error) {
	return &notfoundtx{}, nil
}

type notfoundtx struct{}

func (*notfoundtx) commit() error {
	return nil
}

func (*notfoundtx) rollback() error {
	return nil
}

func (*notfoundtx) addHit(id int64, ip net.IP, agent string, r *string) error {
	return nil
}

func (*notfoundtx) addURL(name, url, user string) error {
	return nil
}

func (*notfoundtx) getURLnID(name string) (string, int64, error) {
	return "", 0, sql.ErrNoRows
}

func (*notfoundtx) getIDnUser(name string) (int64, string, error) {
	return 0, "", sql.ErrNoRows
}

func (*notfoundtx) removeURL(name string) error {
	return nil
}

func (*notfoundtx) urlsForUser(user string) ([]map[string]string, error) {
	return []map[string]string{}, nil
}

// fakedb mocks a somewhat working db
type fakedb struct{}

func (*fakedb) begin() (Tx, error) {
	return &faketx{}, nil
}

type faketx struct {
	notfoundtx
}

func (*faketx) getURLnID(name string) (string, int64, error) {
	return "http://example.com", 0, nil
}

func (*faketx) getIDnUser(name string) (int64, string, error) {
	return 0, "test", nil
}

func (*faketx) removeURL(name string) error {
	return nil
}

func (*faketx) urlsForUser(user string) ([]map[string]string, error) {
	result := []map[string]string{
		{
			"name": "foo",
			"url":  "http://example.com",
			"hits": "0",
		},
		{
			"name": "bar",
			"url":  "http://example.net",
			"hits": "1",
		},
	}
	return result, nil
}

func TestRedirHandler(t *testing.T) {
	// missing dbHandler
	handler := panicHandler(http.HandlerFunc(redirHandler))
	req := httptest.NewRequest("GET", "/", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	// problematic db
	handler = panicHandler(dbHandler(&errdb{},
		http.HandlerFunc(redirHandler)))

	testRequest(t, handler, req, http.StatusInternalServerError)

	// missing URL
	handler = panicHandler(dbHandler(&notfounddb{},
		http.HandlerFunc(redirHandler)))
	req = httptest.NewRequest("GET", "/foo", nil)

	testRequest(t, handler, req, http.StatusNotFound)

	// everything ok
	handler = panicHandler(dbHandler(&fakedb{},
		http.HandlerFunc(redirHandler)))
	req = httptest.NewRequest("GET", "/foo", nil)

	req.Header.Set("Referer", "http://example.org")

	rr, _ := testRequest(t, handler, req, http.StatusMovedPermanently)

	if rr.HeaderMap.Get("Location") != "http://example.com" {
		t.Error("Wrong location header:", rr.HeaderMap.Get("Location"))
	}

	if rr.HeaderMap.Get("Cache-Control") != "private, max-age=90" {
		t.Error("Wrong cache header:", rr.HeaderMap.Get("Cache-Control"))
	}
}

func TestDeleteHandler(t *testing.T) {
	// missing dbHandler
	handler := panicHandler(http.HandlerFunc(redirHandler))
	req := httptest.NewRequest("DELETE", "/foo", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	// missing user
	handler = panicHandler(dbHandler(&errdb{},
		http.HandlerFunc(deleteHandler)))

	testRequest(t, handler, req, http.StatusInternalServerError)

	// empty user
	handler = panicHandler(remoteUserHandler("X-Remote-User",
		dbHandler(&fakedb{}, http.HandlerFunc(deleteHandler))))

	testRequest(t, handler, req, http.StatusBadRequest)

	// wrong user
	handler = panicHandler(staticUserHandler("bar", dbHandler(&fakedb{},
		http.HandlerFunc(deleteHandler))))

	testRequest(t, handler, req, http.StatusForbidden)

	// everythink ok
	handler = panicHandler(staticUserHandler("test", dbHandler(&fakedb{},
		http.HandlerFunc(deleteHandler))))

	testRequest(t, handler, req, http.StatusOK)
}

func TestIndexHandler(t *testing.T) {
	handler := panicHandler(staticUserHandler("test", dbHandler(&fakedb{},
		http.HandlerFunc(indexHandler))))
	req := httptest.NewRequest("GET", "/foo", nil)

	testRequest(t, handler, req, http.StatusMovedPermanently)

	req = httptest.NewRequest("DELETE", "/foo", nil)

	testRequest(t, handler, req, http.StatusOK)

	req = httptest.NewRequest("POST", "/foo", nil)

	testRequest(t, handler, req, http.StatusBadRequest)
}

// postForm is a test helper for POST requests
func postForm(t *testing.T, handler http.Handler, target string,
	values url.Values, code int) (*httptest.ResponseRecorder, string) {
	req := httptest.NewRequest("POST", target,
		strings.NewReader(values.Encode()))

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return testRequest(t, handler, req, code)
}

func TestAdminHandler(t *testing.T) {
	// missing dbHandler
	handler := panicHandler(http.HandlerFunc(adminHandler))
	req := httptest.NewRequest("GET", "/_admin", nil)

	testRequest(t, handler, req, http.StatusInternalServerError)

	// missing user
	handler = panicHandler(dbHandler(&errdb{},
		http.HandlerFunc(adminHandler)))

	testRequest(t, handler, req, http.StatusInternalServerError)

	// ok GET request
	handler = panicHandler(staticUserHandler("test", dbHandler(&fakedb{},
		http.HandlerFunc(adminHandler))))

	testRequest(t, handler, req, http.StatusOK)

	// wrong HTTP method
	handler = panicHandler(staticUserHandler("test", dbHandler(&fakedb{},
		http.HandlerFunc(adminHandler))))
	req = httptest.NewRequest("HEAD", "/_admin", nil)

	testRequest(t, handler, req, http.StatusBadRequest)

	// missing POST form
	req = httptest.NewRequest("POST", "/_admin", nil)

	testRequest(t, handler, req, http.StatusBadRequest)

	// bad URL
	_, body := postForm(t, handler, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {":foo/bar"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if body != "Malformed URL" {
		t.Error("Wrong body", body)
	}

	// missing URL
	_, body = postForm(t, handler, "/_admin", url.Values{
		"name": {"baz"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if body != "Missing URL" {
		t.Error("Wrong body", body)
	}

	// missing name
	_, body = postForm(t, handler, "/_admin", url.Values{
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusBadRequest)

	if body != "Missing name" {
		t.Error("Wrong body", body)
	}

	// missing user
	_, body = postForm(t, handler, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
	}, http.StatusBadRequest)

	if body != "Missing user" {
		t.Error("Wrong body", body)
	}

	// everything ok
	postForm(t, handler, "/_admin", url.Values{
		"name": {"baz"},
		"url":  {"http://example.com"},
		"user": {"test"},
	}, http.StatusSeeOther)
}
