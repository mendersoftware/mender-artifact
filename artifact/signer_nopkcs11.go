// Copyright 2022 Northern.tech AS
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

//go:build nopkcs11
// +build nopkcs11

package artifact

import (
	"github.com/pkg/errors"
)

// PKCS11Signer implements SigningKey interface, allowing non-linux platforms to be compiled.
type PKCS11Signer struct {
}

func NewPKCS11Signer(pkcsKey string) (*PKCS11Signer, error) {
	return nil, errors.New("PKCS#11 is supported only on linux")
}

func (s *PKCS11Signer) Sign(message []byte) ([]byte, error) {
	return nil, errors.New("PKCS#11 is supported only on linux")
}

func (s *PKCS11Signer) Verify(message, sig []byte) error {
	return errors.New("PKCS#11 is supported only on linux")
}
