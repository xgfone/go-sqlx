// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/xgfone/go-sqlx/dialect"
)

// OpenOptions separates pre-open connection settings from post-open pool
// configuration. Use keyed literals so future fields remain source-compatible.
type OpenOptions struct {
	Connection dialect.ConnectionOptions

	// Required lists options that ConfigureConnection must report as handled.
	// By default unsupported options are ignored. Required is an explicit
	// caller policy, checked before invoking the driver opener.
	Required []string

	// Configs configure the pool after opening. Nil uses DefaultConfigs;
	// a non-nil empty slice disables defaults. Open always uses DefaultConfigs.
	Configs []Config
}

// Open opens a database specified by its database driver name and native data
// source name. It is equivalent to OpenWithOptions with an empty OpenOptions:
// the dialect prepares the native DSN with no connection options, and the pool
// is configured using [DefaultConfigs]. Use [OpenWithOptions] for per-pool
// configurations or connection options.
//
// If driverName is empty, dataSourceName may have the form "[dialect+]driver://dsn".
// Open removes this prefix before calling the dialect's ConfigureConnection method
// and [DefaultOpener]. Without a dialect prefix, driver also names the registered
// dialect. With a dialect prefix, only the dialect must be registered in this
// package; driver names the driver used by DefaultOpener.
//
// With no driverName or data source prefix, Open uses [dialect.Default]
// and its Name as the driver name, with dataSourceName as the native DSN.
// An empty default driver name returns an error.
//
// The remaining dsn must use the driver's native format, including any scheme
// it requires. For example, "postgres+pgx://postgres://user:pass@host/db" selects
// the postgres dialect and pgx driver with "postgres://user:pass@host/db" as dsn.
// A nonempty driverName bypasses prefix parsing.
func Open(driverName, dataSourceName string) (*DB, error) {
	return OpenWithOptions(driverName, dataSourceName, OpenOptions{})
}

// OpenWithOptions resolves the dialect and actual driver using Open's rules,
// prepares the native DSN with Dialect.ConfigureConnection, then invokes
// DefaultOpener and applies pool configuration. It remains lazy: callers may
// ping the pool to verify server-side settings. Unsupported options are ignored
// unless listed in Required. Connection options never change binding settings.
// The input option map is shallow-copied for the call; its values must remain
// immutable for the duration of the call. No connection settings are retained.
func OpenWithOptions(driverName, dataSourceName string, options OpenOptions) (*DB, error) {
	d, driverName, dsn, err := resolveOpenConnection(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	prepared, err := d.ConfigureConnection(dialect.ConnectionRequest{
		Driver:  driverName,
		DSN:     dsn,
		Options: maps.Clone(options.Connection),
	})
	if err != nil {
		const msg = "sqlx: configure connection for driver %q: %w"
		return nil, fmt.Errorf(msg, driverName, err)
	}

	for _, key := range options.Required {
		if !slices.Contains(prepared.Handled, key) {
			const msg = "sqlx: driver %q did not handle required connection option %q"
			return nil, fmt.Errorf(msg, driverName, key)
		}
	}

	db, err := DefaultOpener(driverName, prepared.DSN)
	if err != nil {
		return nil, err
	}

	configs := options.Configs
	if configs == nil {
		configs = DefaultConfigs()
	}

	for _, c := range configs {
		c(db)
	}

	return (&DB{Dialect: d}).SetExecutor(db), nil
}

func resolveOpenConnection(driverName, dataSourceName string) (Dialect, string, string, error) {
	var d Dialect
	dialectName := driverName
	if driverName == "" {
		prefix, dsn, ok := strings.Cut(dataSourceName, "://")
		// A delimiter inside native credentials, paths, or options is not a
		// metadata prefix. Do not parse or rewrite the native DSN.
		if ok && !strings.ContainsAny(prefix, ":/@?=()\\") {
			dialectName, driverName, ok = strings.Cut(prefix, "+")
			if !ok {
				driverName = dialectName
			}

			if dialectName == "" || driverName == "" ||
				strings.Contains(driverName, "+") ||
				driverName != strings.TrimSpace(driverName) ||
				dialectName != strings.TrimSpace(dialectName) {
				return nil, "", "", errors.New("sqlx: invalid [dialect+]driver://dsn prefix")
			}

			dataSourceName = dsn
		} else {
			d = dialect.Default()
			driverName = d.Name()
			if strings.TrimSpace(driverName) == "" {
				return nil, "", "", errors.New("sqlx: default dialect has an empty driver name")
			}
		}
	}

	if d == nil {
		d = dialect.Get(dialectName)
		if d == nil {
			return nil, "", "", fmt.Errorf("the dialect '%s' has not been registered", dialectName)
		}
	}

	return d, driverName, dataSourceName, nil
}
