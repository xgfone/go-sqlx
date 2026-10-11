// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ConnectionOptions holds extensible, read-only connection settings. Each
// supported key defines its value type. Unknown keys are ignored.
type ConnectionOptions map[string]any

// OptionLocation requests a driver's time handling and, where supported, the
// database session time zone. Each adapter documents which locations synchronize
// both settings. Its value must be a *time.Location. A nil location is unspecified;
// callers choose their own default before opening a connection.
const OptionLocation = "location"

// ConnectionRequest describes one resolved driver connection. DSN contains only
// the driver's native data source, with no SQLx metadata prefix.
type ConnectionRequest struct {
	Driver  string
	DSN     string
	Options ConnectionOptions
}

// ConnectionResult is the prepared native DSN and options successfully handled
// according to the adapter's documented precedence and validation policy.
// Handled must omit unsupported or unspecified options and not share mutable
// storage with another call's result.
type ConnectionResult struct {
	DSN     string
	Handled []string
}

// ConnectionConfigFunc prepares native connection settings. It follows
// Dialect.ConfigureConnection's contract and must be safe for concurrent calls.
type ConnectionConfigFunc func(ConnectionRequest) (ConnectionResult, error)

type connectionKey struct{ dialect, driver string }

var connectionConfigurers = struct {
	sync.RWMutex
	values map[connectionKey]ConnectionConfigFunc
}{values: make(map[connectionKey]ConnectionConfigFunc)}

// RegisterConnectionConfigurer adds a native driver adapter for a built-in
// dialect's canonical Name and an actual driver name. Both names are
// case-sensitive. Duplicate registrations fail. Register before opening
// connections; the driver must also be registered with database/sql.
// Custom dialects can implement ConfigureConnection directly.
func RegisterConnectionConfigurer(dialectName, driver string, configure ConnectionConfigFunc) error {
	if dialectName == "" || driver == "" ||
		driver != strings.TrimSpace(driver) ||
		dialectName != strings.TrimSpace(dialectName) {
		return errors.New("dialect: invalid connection configurer registration names")
	}
	if configure == nil {
		return errors.New("dialect: nil connection configurer")
	}

	key := connectionKey{dialectName, driver}
	connectionConfigurers.Lock()
	defer connectionConfigurers.Unlock()
	if _, exists := connectionConfigurers.values[key]; exists {
		return fmt.Errorf("dialect: connection configurer for %q/%q already registered", dialectName, driver)
	}

	connectionConfigurers.values[key] = configure
	return nil
}

// UnregisterConnectionConfigurer removes an adapter. Previously prepared pools
// keep their DSNs; subsequently opened pools observe the new registration state.
func UnregisterConnectionConfigurer(dialectName, driver string) bool {
	key := connectionKey{dialectName, driver}
	connectionConfigurers.Lock()
	defer connectionConfigurers.Unlock()
	_, exists := connectionConfigurers.values[key]
	delete(connectionConfigurers.values, key)
	return exists
}

func (d builtin) ConfigureConnection(request ConnectionRequest) (ConnectionResult, error) {
	key := connectionKey{string(d), request.Driver}
	connectionConfigurers.RLock()
	configure := connectionConfigurers.values[key]
	connectionConfigurers.RUnlock()
	if configure != nil {
		return configure(request)
	}
	return ConnectionResult{DSN: request.DSN}, nil
}
