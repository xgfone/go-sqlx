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

import "testing"

var limitOffsetResult string

func BenchmarkLimitOffset(b *testing.B) {
	for _, d := range []Dialect{MySQL, Postgres, SQLite} {
		for _, test := range []struct {
			name string
			page Pagination
		}{
			{"empty", Pagination{}},
			{"limit", Pagination{Limit: 10, HasLimit: true}},
			{"offset", Pagination{Offset: 1000}},
			{"both", Pagination{Limit: 100, HasLimit: true, Offset: 1000}},
		} {
			b.Run(d.Name()+"/"+test.name, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					limitOffsetResult = d.LimitOffset(test.page)
				}
			})
		}
	}
}
