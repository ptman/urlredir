// Copyright (c) 2017-2021 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	nurl "net/url"
	"time"

	_ "github.com/lib/pq"
)

// key is used for storing values in context.
type key int

const (
	// txKey is key for transaction in context.
	txKey key = iota
	// userKey is key for user name in context.
	userKey
)

type errorHandler func(http.ResponseWriter, *http.Request) error

func withError(h errorHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			//nolint:errorlint
			if he, ok := err.(http.Handler); ok {
				he.ServeHTTP(w, r)
			} else {
				handleError(w, err,
					http.StatusInternalServerError)
			}
		}
	}
}

// ServeHTTP implements http.Handler.
func (h errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	withError(h)(w, r)
}

// parseIP returns a parsed IP address if possible.
func parseIP(s string) (net.IP, error) {
	inet, _, err := net.SplitHostPort(s)
	if err != nil {
		inet = s
	}

	ip := net.ParseIP(inet)
	if ip == nil {
		return nil, fmt.Errorf("%w ip: %s", ErrInvalidIP, s)
	}

	return ip, nil
}

// handleError logs error and writes error response to client.
func handleError(w http.ResponseWriter, err error, code int) {
	log.Print(err)
	http.Error(w, http.StatusText(code), code)
}

// panicHandler recovers from panics and returns ISE to clients.
func panicHandler(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch t := r.(type) {
				case error:
					err = t
				default:
					err = fmt.Errorf("%w: %s", ErrUnknown,
						t)
				}
				handleError(w, err,
					http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	}
}

// realIPHandler fixes client IP in request when running behind reverse proxy.
func realIPHandler(header string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		realIP := r.Header.Get(header)
		if realIP != "" {
			r.RemoteAddr = realIP
		}

		next.ServeHTTP(w, r)
	}
}

// staticUserHandler sets a static user name in the context, e.g. for testing.
func staticUserHandler(user string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, userKey,
			user)))
	}
}

// remoteUserHandler sets user name in context based on headers from proxy.
func remoteUserHandler(header string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user := r.Header.Get(header)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, userKey,
			user)))
	}
}

// dbHandler opens transaction in context and rollbacks if there's a panic.
func dbHandler(db DB, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tx, err := db.beginTx(ctx)
		if err != nil {
			panic(err)
		}

		defer func() {
			if r := recover(); r != nil {
				err = tx.rollback()
				if err != nil {
					panic(err)
				}

				panic(r)
			}
		}()

		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, txKey,
			tx)))

		err = tx.commit()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			panic(err)
		}
	}
}

// redirHandler redirects if URL is found in database.
func redirHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	tx, ok := ctx.Value(txKey).(Tx)
	if !ok {
		panic("no tx")
	}

	name := r.URL.Path[1:]
	agent := r.Header.Get("User-Agent")
	referer := r.Header.Get("Referer")

	var referrer *string

	if referer != "" {
		referrer = &referer
	}

	url, urlID, err := tx.getURLnID(name)
	if errors.Is(err, sql.ErrNoRows) {
		if er := tx.rollback(); er != nil {
			panic(er)
		}

		return &HTTPError{Code: http.StatusNotFound}
	} else if err != nil {
		panic(err)
	}

	// 301 seems to be the best combined with cache-control
	w.Header().Set("Cache-Control", "private, max-age=90")
	//nolint:gomnd
	w.Header().Set("Expires", time.Now().Add(90*time.Second).In(
		time.UTC).Format(http.TimeFormat))
	w.Header().Set("Content-Type", "text/html")
	http.Redirect(w, r, url, http.StatusMovedPermanently)

	ip, err := parseIP(r.RemoteAddr)
	if err != nil {
		panic(err)
	}

	if err = tx.addHit(urlID, ip, agent, referrer); err != nil {
		panic(err)
	}

	log.Println(r.RemoteAddr, agent, referer, name, url)

	return nil
}

// deleteHandler removes a specific URL if authorized.
func deleteHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	tx, ok := ctx.Value(txKey).(Tx)
	if !ok {
		panic("no tx")
	}

	user, ok := ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	if user == "" {
		if er := tx.rollback(); er != nil {
			panic(er)
		}

		return &HTTPError{
			Code:    http.StatusBadRequest,
			Message: "Missing user",
		}
	}

	name := r.URL.Path[1:]

	_, urluser, err := tx.getIDnUser(name)
	if errors.Is(err, sql.ErrNoRows) {
		if er := tx.rollback(); er != nil {
			panic(er)
		}

		return &HTTPError{
			Code: http.StatusNotFound,
			Err:  err,
		}
	} else if err != nil {
		panic(err)
	}

	if user != urluser {
		if er := tx.rollback(); er != nil {
			panic(er)
		}

		return &HTTPError{Code: http.StatusForbidden}
	}

	err = tx.removeURL(name)
	if err != nil {
		panic(err)
	}

	log.Println(r.RemoteAddr, "DELETE", name)

	return nil
}

// indexHandler delegates to redirHandler or deleteHandler.
func indexHandler(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "GET":
		return redirHandler(w, r)
	case "DELETE":
		return deleteHandler(w, r)
	default:
		return &HTTPError{
			Code:    http.StatusBadRequest,
			Message: "Bad method",
		}
	}
}

// adminGetHandler serves admin page.
func adminGetHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	tx, ok := ctx.Value(txKey).(Tx)
	if !ok {
		panic("no tx")
	}

	user, ok := ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	urls, err := tx.urlsForUser(user)
	if err != nil {
		panic(err)
	}

	t, err := template.New("adminPage").Parse(adminPage)
	if err != nil {
		panic(err)
	}

	params := map[string]interface{}{
		"path": r.URL.Path,
		"user": user,
		"urls": urls,
	}

	err = t.Execute(w, params)
	if err != nil {
		panic(err)
	}

	return nil
}

// validateAdminForm perform form parameter validation for admin page.
func validateAdminForm(r *http.Request) (string, string, string, error) {
	name := r.FormValue("name")
	url := r.FormValue("url")
	user := r.FormValue("user")

	if name == "" {
		return "", "", "", ErrMissingName
	}

	if url == "" {
		return "", "", "", ErrMissingURL
	}

	if _, err := nurl.Parse(url); err != nil {
		return "", "", "", ErrInvalidURL
	}

	if user == "" {
		return "", "", "", ErrMissingUser
	}

	return name, url, user, nil
}

// adminPostHandler inserts URLs to database.
func adminPostHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	tx, ok := ctx.Value(txKey).(Tx)
	if !ok {
		panic("no tx")
	}

	_, ok = ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	name, url, user, err := validateAdminForm(r)
	if err != nil {
		if er := tx.rollback(); er != nil {
			panic(er)
		}

		return &HTTPError{
			Code:    http.StatusBadRequest,
			Err:     err,
			Message: err.Error(),
		}
	}

	if err := tx.addURL(name, url, user); err != nil {
		panic(err)
	}

	http.Redirect(w, r, "/_admin", http.StatusSeeOther)

	return nil
}

// adminHandler delegates to adminPostHandler or adminGetHandler.
func adminHandler(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case "POST":
		return adminPostHandler(w, r)
	case "GET":
		return adminGetHandler(w, r)
	default:
		return &HTTPError{Code: http.StatusBadRequest}
	}
}
