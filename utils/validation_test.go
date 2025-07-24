// Copyright 2025 Northern.tech AS
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
	"errors"
	"strings"
	"testing"
)

func TestValidateString(t *testing.T) {
	t.Parallel()
	t.Run("ok", func(t *testing.T) {
		testString := "just a regular string #!foobar"
		err := ValidateString(testString)
		if err != nil {
			t.Errorf("unexpected error: %s", err.Error())
		}
	})
	t.Run("string too long", func(t *testing.T) {
		testString := strings.Repeat("GNU's not unix", 100)
		err := ValidateString(testString)
		if err == nil {
			t.Error("expected an error, received none")
		} else if !errors.Is(err, ErrTooManyChars) {
			t.Errorf("unexpected error: %s", err.Error())
		}
	})
	t.Run("string contains invalid characters", func(t *testing.T) {
		testString := "foobar\x00"
		err := ValidateString(testString)
		if err == nil {
			t.Error("expected an error, received none")
		} else if !errors.Is(err, ErrInvalidCharacters) {
			t.Errorf("unexpected error: %s", err.Error())
		}
	})
}
