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

//go:build !linux
// +build !linux

package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPKCS11SignerUnsupportedPlatforms(t *testing.T) {
	testPkcsKey := "pkcs:11some_key"
	expectedErrMsg := "PKCS#11 is supported only on linux"
	signer, err := NewPKCS11Signer(testPkcsKey)
	assert.Nil(t, signer)
	assert.EqualError(t, err, expectedErrMsg)
}

func TestSignUnsupportedPlatforms(t *testing.T) {
	message := []byte("test_msg")
	expectedErrMsg := "PKCS#11 is supported only on linux"
	signer := PKCS11Signer{}
	signed, err := signer.Sign(message)
	assert.Nil(t, signed)
	assert.EqualError(t, err, expectedErrMsg)
}

func TestVerifyUnsupportedPlatforms(t *testing.T) {
	message := []byte("test_msg")
	sig := []byte("test_sig")
	expectedErrMsg := "PKCS#11 is supported only on linux"
	signer := PKCS11Signer{}
	err := signer.Verify(message, sig)
	assert.EqualError(t, err, expectedErrMsg)
}
