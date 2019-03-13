// Copyright 2019 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package utils

import (
	"strings"
	"testing"
)

func verifyContains(t *testing.T, a string, b string) {
	if !strings.Contains(a, b) {
		t.Errorf("GetBinaryPath did not contain '%s' in result '%s'.",
			b, a)
	}
}


func TestGetBinaryPath(t *testing.T) {
	nonexist := "non-existant-command-should-still-be-returned"
	p := GetBinaryPath(nonexist)
	verifyContains(t, p, nonexist)

	// Note: assume /bin/true is available on every build-system always.

	alwaysFoundCommand := "true"
	p = GetBinaryPath(alwaysFoundCommand)
	verifyContains(t, p, alwaysFoundCommand)

	alwaysFoundCommandFullPath := "/bin/true"
	p = GetBinaryPath(alwaysFoundCommandFullPath)
	verifyContains(t, p, alwaysFoundCommandFullPath)
}
