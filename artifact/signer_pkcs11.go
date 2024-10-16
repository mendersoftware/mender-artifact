// Copyright 2024 Northern.tech AS
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

//go:build !nopkcs11
// +build !nopkcs11

package artifact

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

const (
	pkcs11URIPrefix = "pkcs11:"
)

type PKCS11Signer struct {
	Key *rsa.PrivateKey
}

func NewPKCS11Signer(pkcsKey string) (*PKCS11Signer, error) {
	if len(pkcsKey) == 0 {
		return nil, errors.New("PKCS#11 signer: missing key")
	}

	key, err := loadPrivateKey(pkcsKey)
	if err != nil {
		return nil, errors.New("PKCS#11: failed to load private key: " + err.Error())
	}

	return &PKCS11Signer{
		Key: key,
	}, nil
}

func (s *PKCS11Signer) Sign(message []byte) ([]byte, error) {
	hashed := sha256.Sum256(message)

	// Sign the hashed message using RSA PKCS1v15
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.Key, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, errors.New("PKCS#11 signer: error signing image")
	}

	// Encode signature in base64
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(sig)))
	base64.StdEncoding.Encode(enc, sig)
	return enc, nil
}

func (s *PKCS11Signer) Verify(message, sig []byte) error {
	// Decode the signature from base64
	dec := make([]byte, base64.StdEncoding.DecodedLen(len(sig)))
	decLen, err := base64.StdEncoding.Decode(dec, sig)
	if err != nil {
		return errors.New("signer: error decoding signature")
	}

	// Hash the message
	hashed := sha256.Sum256(message)

	// Verify the signature using RSA PKCS1v15
	err = rsa.VerifyPKCS1v15(&s.Key.PublicKey, crypto.SHA256, hashed[:], dec[:decLen])
	if err != nil {
		return errors.New("failed to verify PKCS#11 signature")
	}

	return nil
}

func loadPrivateKey(keyFile string) (*rsa.PrivateKey, error) {
	if strings.HasPrefix(keyFile, pkcs11URIPrefix) {
		keyData, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}

		block, _ := pem.Decode(keyData)
		if block == nil || block.Type != "RSA PRIVATE KEY" {
			return nil, errors.New("failed to decode PEM block containing private key")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}

		return key, nil
	}
	return nil, errors.New("PKCS#11 URI prefix not found")
}
