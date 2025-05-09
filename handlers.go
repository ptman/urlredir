// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"

	_ "github.com/lib/pq"
)

// ctxKey is used for storing values in context.
type ctxKey int

const (
	// txKey is key for transaction in context.
	txKey ctxKey = iota
	// userKey is key for user name in context.
	userKey
)

// must panics if error isn't nil.
// x := must(funcThatReturnsValAndErr()).
func must[T any](v T, err error) T { //nolint:ireturn
	if err != nil {
		panic(err)
	}

	return v
}

// getTx returns the transaction from the context.
func getTx(ctx context.Context) (*sql.Tx, error) {
	tx, ok := ctx.Value(txKey).(*sql.Tx)
	if !ok {
		return nil, &HTTPError{
			Code:    http.StatusInternalServerError,
			Err:     ErrNoTx,
			Message: "no tx",
		}
	}

	return tx, nil
}

// getUser returns the username from the context.
func getUser(ctx context.Context) (string, error) {
	user, ok := ctx.Value(userKey).(string)
	if !ok {
		return "", &HTTPError{
			Code:    http.StatusInternalServerError,
			Err:     ErrMissingUser,
			Message: "no user",
		}
	}

	return user, nil
}

// errorHandler is a slight modification to standard http.HandlerFunc.
type errorHandler func(http.ResponseWriter, *http.Request) error

// withError turns errorHandler to a normal http.Handler.
func withError(h errorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			if he, ok := err.(http.Handler); ok {
				he.ServeHTTP(w, r)
			} else {
				handleError(w, err,
					http.StatusInternalServerError)
			}
		}
	})
}

// ServeHTTP implements http.Handler.
func (h errorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	withError(h).ServeHTTP(w, r)
}

// middleware is an http handler that returns another http handler.
type middleware func(http.Handler) http.Handler

// chain is a middleware chain.
// use chain{first, second, last}.apply(handler) instead of
// first(second(last(handler))).
type chain []middleware

// apply applies the middleware chain to an http handler.
func (c chain) apply(h http.Handler) http.Handler {
	for _, m := range slices.Backward(c) {
		h = m(h)
	}

	return h
}

// applyE applies the middleware chain to an errorHandler.
func (c chain) applyE(h errorHandler) http.Handler {
	return c.apply(withError(h))
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
	slog.Error("error", slog.Int("status", code), slog.Any("err", err))
	http.Error(w, http.StatusText(code), code)
}

// loggerMiddleware logs HTTP requests.
func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		slog.Info("req", slog.String("addr", r.RemoteAddr),
			slog.String("method", r.Method),
			slog.String("url", r.URL.String()),
			slog.String("proto", r.Proto),
			slog.String("referer", r.Referer()),
			slog.String("userAgent", r.UserAgent()),
			slog.Any("duration", time.Since(start)),
		)
	})
}

// panicMiddleware recovers from panics and returns ISE to clients.
func panicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
}

// realIPMiddleware fixes client IP in request when running behind reverse proxy.
func realIPMiddleware(header string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter,
			r *http.Request,
		) {
			realIP := r.Header.Get(header)
			if realIP != "" {
				r.RemoteAddr = realIP
			}

			next.ServeHTTP(w, r)
		})
	}
}

// staticUserMiddleware sets a static user name in the context, e.g. for testing.
func staticUserMiddleware(user string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter,
			r *http.Request,
		) {
			ctx := r.Context()
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx,
				userKey, user)))
		})
	}
}

// remoteUserMiddleware sets user name in context based on headers from proxy.
func remoteUserMiddleware(header string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter,
			r *http.Request,
		) {
			ctx := r.Context()
			user := r.Header.Get(header)
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx,
				userKey, user)))
		})
	}
}

// beginner is an interface that can start a transaction (e.g. pool and conn).
type beginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// dbMiddleware opens transaction in context and rollbacks if there's a panic.
func dbMiddleware(db beginner) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter,
			r *http.Request,
		) {
			ctx := r.Context()

			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				panic(err)
			}

			defer func() {
				if r := recover(); r != nil {
					err = tx.Rollback()
					if err != nil {
						panic(err)
					}

					panic(r)
				}
			}()

			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx,
				txKey, tx)))

			err = tx.Commit()
			if err != nil && !errors.Is(err, sql.ErrTxDone) {
				panic(err)
			}
		})
	}
}

// redirHandler redirects if URL is found in database.
func redirHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	tx := must(getTx(ctx))
	name := r.PathValue("name")
	agent := r.UserAgent()
	referer := r.Referer()

	slog.Debug("REDIR", slog.String("name", name))

	var referrer *string

	if referer != "" {
		referrer = &referer
	}

	u, urlID, err := getURLnID(ctx, tx, name)
	if errors.Is(err, sql.ErrNoRows) {
		//nolint:exhaustruct
		return &HTTPError{Code: http.StatusNotFound}
	} else if err != nil {
		return err
	}

	// 301 seems to be the best combined with cache-control
	w.Header().Set("Cache-Control", "private, max-age=90")
	//nolint:mnd
	w.Header().Set("Expires", time.Now().Add(90*time.Second).In(
		time.UTC).Format(http.TimeFormat))
	w.Header().Set("Content-Type", "text/html")
	http.Redirect(w, r, u, http.StatusMovedPermanently)

	ip, err := parseIP(r.RemoteAddr)
	if err != nil {
		return err
	}

	if err = addHit(ctx, tx, urlID, ip, agent, referrer); err != nil {
		return err
	}

	slog.InfoContext(ctx, "redirect", slog.String("agent", agent),
		slog.String("referer", referer), slog.String("name", name),
		slog.String("url", u), slog.String("remote", r.RemoteAddr))

	return nil
}

// deleteHandler removes a specific URL if authorized.
func deleteHandler(_ http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	tx := must(getTx(ctx))
	user := must(getUser(ctx))

	if user == "" {
		return &HTTPError{ //nolint:exhaustruct
			Code:    http.StatusBadRequest,
			Message: "Missing user",
		}
	}

	name := r.PathValue("name")

	_, urluser, err := getIDnUser(ctx, tx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return &HTTPError{ //nolint:exhaustruct
			Code: http.StatusNotFound,
			Err:  err,
		}
	} else if err != nil {
		return err
	}

	if user != urluser {
		//nolint:exhaustruct
		return &HTTPError{Code: http.StatusForbidden}
	}

	err = removeURL(ctx, tx, name)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "DELETE", slog.String("remote", r.RemoteAddr),
		slog.String("name", name))

	return nil
}

// adminGetHandler serves admin page.
func adminGetHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	tx := must(getTx(ctx))
	user := must(getUser(ctx))

	urls, err := urlsForUser(ctx, tx, user)
	if err != nil {
		return err
	}

	t, err := template.New("adminPage").Parse(adminPage)
	if err != nil {
		return fmt.Errorf("failed parsing template: %w", err)
	}

	params := map[string]interface{}{
		"path": r.URL.Path,
		"user": user,
		"urls": urls,
	}

	err = t.Execute(w, params)
	if err != nil {
		return fmt.Errorf("failed executing template: %w", err)
	}

	return nil
}

// validateAdminForm perform form parameter validation for admin page.
func validateAdminForm(r *http.Request) (string, string, string, error) {
	name := r.FormValue("name")
	u := r.FormValue("url")
	user := r.FormValue("user")

	if name == "" {
		return "", "", "", ErrMissingName
	}

	if u == "" {
		return "", "", "", ErrMissingURL
	}

	if _, err := url.Parse(u); err != nil {
		return "", "", "", ErrInvalidURL
	}

	if user == "" {
		return "", "", "", ErrMissingUser
	}

	return name, u, user, nil
}

// adminPostHandler inserts URLs to database.
func adminPostHandler(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	tx := must(getTx(ctx))
	must(getUser(ctx))

	name, u, user, err := validateAdminForm(r)
	if err != nil {
		return &HTTPError{
			Code:    http.StatusBadRequest,
			Err:     err,
			Message: err.Error(),
		}
	}

	if err := addURL(ctx, tx, name, u, user); err != nil {
		return err
	}

	http.Redirect(w, r, "/_admin", http.StatusSeeOther)

	return nil
}
