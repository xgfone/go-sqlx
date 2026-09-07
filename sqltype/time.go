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

package sqltype

// MySQL zero-date spellings, for adapters that explicitly accept zero dates.
const (
	DateZero     = "0000-00-00"
	TimeZero     = "00:00:00"
	DateTimeZero = "0000-00-00 00:00:00"
)
