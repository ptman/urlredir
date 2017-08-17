// Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.

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

// key is used for storing values in context
type key int

const (
	// txKey is key for transaction in context
	txKey key = iota
	// userKey is key for user name in context
	userKey
)

// parseIP returns a parsed IP address if possible
func parseIP(s string) (net.IP, error) {
	inet, _, err := net.SplitHostPort(s)
	if err != nil {
		inet = s
	}
	ip := net.ParseIP(inet)
	if ip == nil {
		return nil, fmt.Errorf("Couldn't parse IP: %s", s)
	}
	return ip, nil
}

// handleError logs error and writes error response to client
func handleError(w http.ResponseWriter, err error, code int) {
	log.Print(err)
	http.Error(w, http.StatusText(code), code)
}

// panicHandler recovers from panics and returns ISE to clients
func panicHandler(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch t := r.(type) {
				case error:
					err = t
				default:
					err = errors.New(fmt.Sprint(t))
				}
				handleError(w, err,
					http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// realIPHandler fixes client IP in request when running behind reverse proxy
func realIPHandler(header string, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realIP := r.Header.Get(header)
		if realIP != "" {
			r.RemoteAddr = realIP
		}

		next.ServeHTTP(w, r)
	})
}

// staticUserHandler sets a static user name in the context, e.g. for testing
func staticUserHandler(user string, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, userKey,
			user)))
	})
}

// remoteUserHandler sets user name in context based on headers from proxy
func remoteUserHandler(header string, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		user := r.Header.Get(header)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, userKey,
			user)))
	})
}

// dbHandler opens transaction in context and rollbacks if there's a panic
func dbHandler(db Db, next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if err != nil && err != sql.ErrTxDone {
			panic(err)
		}
	})
}

// redirHandler redirects if URL is found in database
func redirHandler(w http.ResponseWriter, r *http.Request) {
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
	if err == sql.ErrNoRows {
		if er := tx.rollback(); er != nil {
			panic(er)
		}
		http.NotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	// 301 seems to be the best combined with cache-control
	w.Header().Set("Cache-Control", "private, max-age=90")
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
}

// deleteHandler removes a specific URL if authorized
func deleteHandler(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Missing user", http.StatusBadRequest)
		return
	}

	name := r.URL.Path[1:]

	_, urluser, err := tx.getIDnUser(name)
	if err == sql.ErrNoRows {
		if er := tx.rollback(); er != nil {
			panic(er)
		}
		http.NotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if user != urluser {
		if er := tx.rollback(); er != nil {
			panic(er)
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	err = tx.removeURL(name)
	if err != nil {
		panic(err)
	}

	log.Println(r.RemoteAddr, "DELETE", name)
}

// indexHandler delegates to redirHandler or deleteHandler
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		redirHandler(w, r)
		return
	}
	if r.Method == "DELETE" {
		deleteHandler(w, r)
		return
	}
	http.Error(w, "Bad method", http.StatusBadRequest)
}

// adminGetHandler serves admin page
func adminGetHandler(w http.ResponseWriter, r *http.Request) {
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
}

// validateAdminForm perform form parameter validation for admin page
func validateAdminForm(r *http.Request) (string, string, string, error) {
	name := r.FormValue("name")
	url := r.FormValue("url")
	user := r.FormValue("user")

	if name == "" {
		return "", "", "", errors.New("Missing name")
	}
	if url == "" {
		return "", "", "", errors.New("Missing URL")
	}
	if _, err := nurl.Parse(url); err != nil {
		return "", "", "", errors.New("Malformed URL")
	}
	if user == "" {
		return "", "", "", errors.New("Missing user")
	}

	return name, url, user, nil
}

// adminPostHandler inserts URLs to database
func adminPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, ok := ctx.Value(txKey).(Tx)
	if !ok {
		panic("no tx")
	}

	user, ok := ctx.Value(userKey).(string)
	if !ok {
		panic("no user")
	}

	name, url, user, err := validateAdminForm(r)
	if err != nil {
		if er := tx.rollback(); er != nil {
			panic(er)
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := tx.addURL(name, url, user); err != nil {
		panic(err)
	}

	http.Redirect(w, r, "/_admin", http.StatusSeeOther)
}

// adminHandler delegates to adminPostHandler or adminGetHandler
func adminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		adminPostHandler(w, r)
		return
	}
	if r.Method == "GET" {
		adminGetHandler(w, r)
		return
	}
	http.Error(w, "Bad method", http.StatusBadRequest)
}
