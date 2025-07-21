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
	"unicode"
)

var (
	ErrTooManyChars      = errors.New("too many characters")
	ErrInvalidCharacters = errors.New("string contains invalid characters")
)

const MaxStringLength = 256

func ValidateString(arg string) error {
	if len(arg) > MaxStringLength {
		return ErrTooManyChars
	}
	i := strings.IndexFunc(arg, func(r rune) bool {
		return !unicode.IsPrint(r)
	})
	if i >= 0 {
		return ErrInvalidCharacters
	}
	return nil
}
