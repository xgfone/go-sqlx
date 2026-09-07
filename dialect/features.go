// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dialect

// Feature identifies a SQL grammar extension used by the builders.
type Feature uint8

const (
	UpdateFrom Feature = iota
	UpdateJoinBeforeSet
	MultiTableUpdate
	MultiTableDelete
	InsertIgnore
	ReplaceInto
	FullJoin
)

// FeatureDialect opts in to grammar extensions beyond single-table DML.
type FeatureDialect interface {
	Supports(Feature) bool
}

// Supports reports whether d explicitly supports a grammar extension.
func Supports(d Dialect, feature Feature) bool {
	f, ok := d.(FeatureDialect)
	return ok && f.Supports(feature)
}

func (d builtin) Supports(feature Feature) bool {
	switch feature {
	case UpdateFrom, FullJoin:
		return d == "postgres" || d == "sqlite3"

	case UpdateJoinBeforeSet, MultiTableUpdate, MultiTableDelete, InsertIgnore:
		return d == "mysql"

	case ReplaceInto:
		return d == "mysql" || d == "sqlite3"

	default:
		return false
	}
}
