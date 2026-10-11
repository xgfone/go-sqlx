// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"runtime"
	"slices"
	"sync/atomic"
	"time"
)

// Config is used to configure the [DB].
type Config func(db *sql.DB)

// Opener is used to open a *[sql.DB].
type Opener func(driverName, dataSourceName string) (*sql.DB, error)

var defaultConfigs atomic.Value

func init() {
	SetDefaultConfigs(MaxOpenConns(0), ConnMaxIdleTime(time.Minute*5))
}

// DefaultConfigs returns a copy of the configurations.
// Initially these are [MaxOpenConns](0) and [ConnMaxIdleTime](5 * time.Minute).
//
// DefaultConfigs and [SetDefaultConfigs] are safe for concurrent use.
func DefaultConfigs() []Config {
	return slices.Clone(defaultConfigs.Load().([]Config))
}

// SetDefaultConfigs replaces the configurations returned by [DefaultConfigs].
// The input slice is copied. Calling it with no configs, including an expanded
// nil slice, disables default configuration. A nil Config panics, leaving the
// previous defaults unchanged.
//
// Config functions must be safe for concurrent calls on different databases.
//
// SetDefaultConfigs and [DefaultConfigs] are safe for concurrent use.
func SetDefaultConfigs(configs ...Config) {
	for _, c := range configs {
		if c == nil {
			panic("sqlx: nil default config")
		}
	}
	defaultConfigs.Store(slices.Clone(configs))
}

// DefaultOpener is used to open a *[sql.DB].
var DefaultOpener Opener = sql.Open

// MaxOpenConns returns a [Config] to set the maximum number of the open connection.
//
// If maxnum is equal to 0, it is [runtime.NumCPU]()*2 by default.
func MaxOpenConns(maxnum int) Config {
	if maxnum == 0 {
		maxnum = runtime.NumCPU() * 2
	}
	return func(db *sql.DB) { db.SetMaxOpenConns(maxnum) }
}

// MaxIdleConns returns a [Config] to set the maximum number of the idle connection.
func MaxIdleConns(n int) Config {
	return func(db *sql.DB) { db.SetMaxIdleConns(n) }
}

// ConnMaxLifetime returns a [Config] to set the maximum lifetime of the connection.
func ConnMaxLifetime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxLifetime(d) }
}

// ConnMaxIdleTime returns a [Config] to set the maximum idle time of the connection.
func ConnMaxIdleTime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxIdleTime(d) }
}
