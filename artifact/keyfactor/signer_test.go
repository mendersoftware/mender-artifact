/*
Copyright © 2023 Keyfactor

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package keyfactor

import (
	"crypto/x509"
	"encoding/pem"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

// GetTestSigner
func GetTestSigner(t *testing.T) *SignServerSigner {
	// Get the signer name from the environment
	workerName := os.Getenv("SIGNSERVER_WORKER_NAME")
	if workerName == "" {
		t.Log("Skipping test until MEN-6895 is resolved")
		t.Skip("SIGNSERVER_WORKER_NAME is not set")
	}

	// Use NewSignServerSigner to create a new SignServerSigner
	signer, err := NewSignServerSigner(workerName)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the signer is not nil
	if signer == nil {
		t.Fatal("signer is nil")
	}

	// Check that the signer has a properly configured client
	assert.NotNil(t, signer.client)

	// Check that the signer has a properly configured worker name
	assert.Equal(t, workerName, signer.workerName)

	// Return the test signer
	return signer
}

func TestSignServerSigner_Sign(t *testing.T) {
	// Get a test signer
	signer := GetTestSigner(t)

	msg := []byte("some msg")

	// Sign the message
	signature, err := signer.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	if len(signature) == 0 {
		t.Fatal("signature is empty")
	}
}

func TestSignServerSigner_getPublicKey(t *testing.T) {
	// Get a test signer
	signer := GetTestSigner(t)

	keyPem, err := signer.getPublicKey()
	if err != nil {
		t.Fatal(err)
	}

	// Decode the PEM block containing the public key
	block, _ := pem.Decode(keyPem)
	if block == nil {
		t.Fatal("failed to decode PEM block containing public key")
	}

	// Try to parse the key from the PEM block - If the key is invalid, this will fail
	_, err = x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignServerSigner_SignAndVerify(t *testing.T) {
	// Get a test signer
	signer := GetTestSigner(t)

	msg := []byte("some msg")

	// Sign the message
	signature, err := signer.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	if len(signature) == 0 {
		t.Fatal("signature is empty")
	}

	// Verify the signature
	err = signer.Verify(msg, signature)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSignServerSigner_SignAndVerifyInvalidSignature(t *testing.T) {
	// Get a test signer
	signer := GetTestSigner(t)

	msg := []byte("some msg")

	// Sign the message
	signature, err := signer.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	if len(signature) == 0 {
		t.Fatal("signature is empty")
	}

	// Verify the signature
	err = signer.Verify(msg, []byte("invalid signature"))
	if err == nil {
		t.Fatal("expected error")
	}
}
