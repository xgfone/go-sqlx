// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// SetConnURLLocation sets the argument "loc" in the connection url if missing.
//
// If loc is nil, return connURL unchanged.
func SetConnURLLocation(connURL string, loc *time.Location) string {
	if loc == nil {
		return connURL
	}

	if index := strings.IndexByte(connURL, '?') + 1; index > 0 {
		query, err := url.ParseQuery(connURL[index:])
		if err == nil && query.Get("loc") == "" {
			query.Set("loc", loc.String())
			return connURL[:index] + query.Encode()
		}
		return connURL
	}

	return connURL + "?loc=" + url.QueryEscape(loc.String())
}

// Config is used to configure the DB.
type Config func(db *sql.DB)

// Opener is used to open a *sql.DB.
type Opener func(driverName, dataSourceName string) (*sql.DB, error)

// DefaultConfigs is the default configs.
var DefaultConfigs = []Config{MaxOpenConns(0), ConnMaxIdleTime(time.Minute * 5)}

// DefaultOpener is used to open a *sql.DB.
var DefaultOpener Opener = sql.Open

// MaxOpenConns returns a Config to set the maximum number of the open connection.
//
// If maxnum is equal to 0, it is runtime.NumCPU()*2 by default.
func MaxOpenConns(maxnum int) Config {
	if maxnum == 0 {
		maxnum = runtime.NumCPU() * 2
	}
	return func(db *sql.DB) { db.SetMaxOpenConns(maxnum) }
}

// MaxIdleConns returns a Config to set the maximum number of the idle connection.
func MaxIdleConns(n int) Config {
	return func(db *sql.DB) { db.SetMaxIdleConns(n) }
}

// ConnMaxLifetime returns a Config to set the maximum lifetime of the connection.
func ConnMaxLifetime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxLifetime(d) }
}

// ConnMaxIdleTime returns a Config to set the maximum idle time of the connection.
func ConnMaxIdleTime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxIdleTime(d) }
}
