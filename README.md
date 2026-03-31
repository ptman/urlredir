# urlredir

A (intentionally) simple URL redirector.

This project exists as an exercise in building well-designed and implemented Go
software. I'm less interested in new features than improving the codebase
quality. The codebase has decent test coverage and is written with minimal
dependencies outside of the standard library.

## Building

Read Makefile and build:

    make

## Run

Edit config:

    cp config.json.sample config.json
    $EDITOR config.json

Run:

    make run  # or ./urlredir

## Testing

Unit tests run in short mode and work without local Postgres/config:

    make test

Integration tests require config and a reachable Postgres database:

    make test-all

Coverage needs the config (for database connection details):

    make cover
