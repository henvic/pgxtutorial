# pgxtutorial
[![GoDoc](https://godoc.org/github.com/henvic/pgxtutorial?status.svg)](https://godoc.org/github.com/henvic/pgxtutorial) [![Build Status](https://github.com/henvic/pgxtutorial/workflows/Integration/badge.svg)](https://github.com/henvic/pgxtutorial/actions?query=workflow%3AIntegration) [![Coverage Status](https://coveralls.io/repos/henvic/pgxtutorial/badge.svg)](https://coveralls.io/r/henvic/pgxtutorial)

This is an accompanying repository of the article [Back to basics: Writing an application using Go and PostgreSQL](https://henvic.dev/posts/go-postgres) by [Henrique Vicente](https://henvic.dev/). Feel free to open issues to ask any questions or comment on anything.

## Environment variables
pgxtutorial uses the following environment variables:

| Environment Variable | Description |
| - | - |
| PostgreSQL environment variables | Please check https://www.postgresql.org/docs/current/libpq-envars.html |
| INTEGRATION_TESTDB | When running go test, database tests will only run if `INTEGRATION_TESTDB=true` |

## tl;dr
To play with it install [Go](https://go.dev/) on your system.
You'll need to connect to a [PostgreSQL](https://www.postgresql.org/) database.
You can check if a connection is working by calling `psql`.

To run tests:

```sh
# Run all tests passing INTEGRATION_TESTDB explicitly
$ INTEGRATION_TESTDB=true go test -v ./...
```

To run application:

```sh
# Create a database
$ psql -c "CREATE DATABASE pgxtutorial;"
# Set the environment variable PGDATABASE
$ export PGDATABASE=pgxtutorial
# Run migrations
$ tern migrate -m ./migrations
# Execute application
$ go run ./cmd/pgxtutorial
2021/11/22 07:21:21 HTTP server listening at localhost:8080
2021/11/22 07:21:21 gRPC server listening at 127.0.0.1:8082
```

## See also
* [pgtools](https://github.com/henvic/pgtools/)
