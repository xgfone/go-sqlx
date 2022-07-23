// Copyright 2022 xgfone
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

package sqlx

import "testing"

func TestTagContainAttr(t *testing.T) {
	if !tagContainAttr("abc", "abc") {
		t.Error("not expect false")
	}

	if !tagContainAttr(",abc", "abc") {
		t.Error("not expect false")
	}

	if !tagContainAttr("abc,rst", "abc") {
		t.Error("not expect false")
	}

	if !tagContainAttr("abc,rst,xyz", "rst") {
		t.Error("not expect false")
	}

	if !tagContainAttr("abc,rst,xyz", "xyz") {
		t.Error("not expect false")
	}
}
