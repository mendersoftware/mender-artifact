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

package vault

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"os"
	"testing"

	"github.com/EcoG-io/mender-artifact/artifact"
	vault "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	PublicRSAKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvTNpmusLgLZgfNI9Q3hw
No77GhlhMSA6eABo6VySYZLgREgKTuLJ3XCoPjzwZNAZ4mEDKEmI+Q53Pl+09SEI
sWnuqNGByJbsfjLufPQ44hXgu3NuTafWzJWoWlDf7UbQxB5NFmKsxm2630Dn3n8q
ekfFeGh3GqGQmAu0wq/atafqXbLQOj6EcVuWz9lXOuh69FZ8dMPIKsQusYhKQID3
3Bqn3tiL3xLSk0BXTPPfgBHB6VeL5nx6rUeG9nktX/SZeaLmQwrvEahy2IhomW7T
ofqIfhUwCb9gDSOR3qj04OCxV/wpRn8DkKO5priruZWxyKSYOSpJwAxmeKjRt0F8
twIDAQAB
-----END PUBLIC KEY-----`
	PublicECKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEjcVaILqOxs7FJdHGt8tmegcTPhWM
dZRUTbT33lRY6f4z6ZNM6TiPNSr0paHZ+j/kFTbbrCVZ7cFxZVGVNLHU+Q==
-----END PUBLIC KEY-----`
	PrivateRSAKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpgIBAAKCAQEAvTNpmusLgLZgfNI9Q3hwNo77GhlhMSA6eABo6VySYZLgREgK
TuLJ3XCoPjzwZNAZ4mEDKEmI+Q53Pl+09SEIsWnuqNGByJbsfjLufPQ44hXgu3Nu
TafWzJWoWlDf7UbQxB5NFmKsxm2630Dn3n8qekfFeGh3GqGQmAu0wq/atafqXbLQ
Oj6EcVuWz9lXOuh69FZ8dMPIKsQusYhKQID33Bqn3tiL3xLSk0BXTPPfgBHB6VeL
5nx6rUeG9nktX/SZeaLmQwrvEahy2IhomW7TofqIfhUwCb9gDSOR3qj04OCxV/wp
Rn8DkKO5priruZWxyKSYOSpJwAxmeKjRt0F8twIDAQABAoIBAQCbzI1m66ySNhxo
TPvz5maJFt6BlGqreH2NOdEqcXd87+TLdYM/iJNwTQfOEIJokdDu0LI3563qYVYi
P8+Ul7o/1hqYW8WCt31RQoGO1dFNo3RnB9vKCK7h009J6BUtn8Xj6YvTJjheQhfD
JgCKAK+q+BUNXQDPJkIaYnFcbFEuigSuXtR5FQvTlfp97gc5P3anIeT9ATtbIkhb
E3qUxxoQzNZWZC12eXO4r4zA5YAiWPEuvJI5daJNto+lm2UivaNWahNB1BfpinI+
lr2TBdUKqdxL6ObC6dUBMMlsFFQSPfcmVJWf9bLjNdq/xW93zk70IfZWyaxTzxXC
UAHATxfJAoGBAPwZVLlFZOpMkrvZJiGytenJIm/+PNfXGvQm8NKfUWOU1dEDKi7x
1eycUHDf+j5Pdh/dY/HNKkEUgNeEa93qaCT092J1mPE55qGV4DZgxskR8spra7QC
bMQ8Hfv1ZQYtIqZHz33aaYlr4tmNjTfGS6lK2uxmMewq1R0IRySwpB+dAoGBAMAg
6ngMt7C1/3ClGPRnwKTv+yrI6eFBKGdA0vkgAjFcGAyXzoGv076/jv60zVaQS9ji
eqYYbhhRiWouzlNX9EO8Ad/WzYV14AkRsq7/xzr6hUrl/rhCbPXARK2GkgRK3qsJ
SbmA5Ihw+u2hAzFJbhTLVRu1chndeU+W8F+t5l9jAoGBAMapO4/ItK7CavtnMtpp
V1uFKgMxSUcZ9t6h9TM1Y1DjD9/m644U+2y6/dUFW9FQkxinQURiVkL04ldzvgEh
4LIG7RAE9eJaq3l4fzi66Mu4vihvoG85XfcCHOrZxaOpW93HRya5QGOPxjOEjd1/
AU7Gc2DJY9vlIQ4A4Pdzz9ItAoGBAJQCB366FVxdqE3n8cR+lQq7ERvRsVLlNjHs
31opzWanEqPI4r5HbHDa81bGhBU2jiejuWZxFYdIcPrK2gmcjUEM+citmqBAwXlb
F/L2ek22Jq8fZU4fZf8fwgiHzb7eypCqVBBC+ksd9kDPtDzo25PLXGI/Moo4crbc
iYq71egPAoGBAN/gfUZE20KRk2AgTie01ydoQB0SpvIYj5ZVzcV2z341HZsYQbcD
yG6mHrUFXGSL+FB6Pnc6tVbNXhzL532sBoh4hZKzxiUph0UBP+OcY0fTp5Tgc6dh
1L1ls+PYSgNgaZ3Tt8/SfA0retntmk6AowRMxYYzqQJVtCbBFClls+km
-----END RSA PRIVATE KEY-----`
	PrivateECKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIJ9sO+53ogvHC7BQoOFdUF1+VdQ44XrYJCfzC+Jd5SAmoAoGCCqGSM49
AwEHoUQDQgAEjcVaILqOxs7FJdHGt8tmegcTPhWMdZRUTbT33lRY6f4z6ZNM6TiP
NSr0paHZ+j/kFTbbrCVZ7cFxZVGVNLHU+Q==
-----END EC PRIVATE KEY-----`
)

func TestVaultSignerCreation(t *testing.T) {
	// Test normal vault client creation
	os.Setenv("VAULT_TOKEN", "sometoken")
	os.Setenv("VAULT_MOUNT_PATH", "some-mount-path")
	client, err := NewVaultSigner("some-name")
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.IsType(t, &VaultSigner{}, client)
	assert.IsType(t, &vault.Logical{}, client.vaultClient)

	// Test error handling of vault client creation
	os.Setenv("VAULT_TOKEN", "")
	os.Setenv("VAULT_MOUNT_PATH", "1234")
	client, err = NewVaultSigner("some-name")
	assert.Error(t, err)
	assert.Nil(t, client)

	os.Setenv("VAULT_TOKEN", "1234")
	os.Setenv("VAULT_MOUNT_PATH", "")
	client, err = NewVaultSigner("some-name")
	assert.Error(t, err)
	assert.Nil(t, client)

	os.Setenv("VAULT_TOKEN", "1234")
	os.Setenv("VAULT_MOUNT_PATH", "1234")
	os.Setenv("VAULT_KEY_VERSION", "2a")
	client, err = NewVaultSigner("some-name")
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestGetKeyType(t *testing.T) {
	// Test error handling of key information retrieval
	vaultSigner := VaultSigner{vaultClient: &fakeVaultClient{
		keyNotFound: true,
		keyType:     "rsa-2048",
		privateKey:  PrivateRSAKey,
		publicKey:   PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err := vaultSigner.Sign([]byte("some-message"))
	assert.Error(t, err)
	assert.Nil(t, signature)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		genericVaultReadError: true,
		keyType:               "rsa-2048",
		privateKey:            PrivateRSAKey,
		publicKey:             PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Error(t, err)
	assert.Nil(t, signature)

	// Test unsupported key type
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:    "12345",
		privateKey: PrivateRSAKey,
		publicKey:  PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Error(t, err)
	assert.Nil(t, signature)

	// Test invalid response data handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:                "rsa-2048",
		invalidKeyTypeResponse: true,
		privateKey:             PrivateRSAKey,
		publicKey:              PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Error(t, err)
	assert.Nil(t, signature)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:                "rsa-2048",
		invalidKeyTypeResponse: true,
		typeAssertionError:     true,
		privateKey:             PrivateRSAKey,
		publicKey:              PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Error(t, err)
	assert.Nil(t, signature)
}

func TestVaultRSASign(t *testing.T) {
	// Sign test message with fakeVaultClient
	vaultSigner := VaultSigner{vaultClient: &fakeVaultClient{
		keyType:    "rsa-2048",
		privateKey: PrivateRSAKey,
		publicKey:  PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err := vaultSigner.Sign([]byte("some-message"))
	assert.NoError(t, err)
	assert.NotNil(t, signature)

	// Check if signature is valid
	pkiVerifier, err := artifact.NewPKIVerifier([]byte(PublicRSAKey))
	assert.NoError(t, err)
	err = pkiVerifier.Verify([]byte("some-message"), signature)
	assert.NoError(t, err)

	// Test signer error handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		genericVaultWriteError: true,
		keyType:                "rsa-2048",
		privateKey:             PrivateRSAKey,
		publicKey:              PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)

	// Test invalid signature response handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		invalidSignatureResponse: true,
		keyType:                  "rsa-2048",
		privateKey:               PrivateRSAKey,
		publicKey:                PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		typeAssertionError:       true,
		invalidSignatureResponse: true,
		keyType:                  "rsa-2048",
		privateKey:               PrivateRSAKey,
		publicKey:                PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)
}

func TestVaultECDSASign(t *testing.T) {
	// Perform normal sign operation
	vaultSigner := VaultSigner{vaultClient: &fakeVaultClient{
		keyType:    "ecdsa-p256",
		privateKey: PrivateECKey,
		publicKey:  PublicECKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err := vaultSigner.Sign([]byte("some-message"))
	assert.NoError(t, err)
	assert.NotNil(t, signature)

	// Check if signature is valid
	pkiVerifier, err := artifact.NewPKIVerifier([]byte(PublicECKey))
	assert.NoError(t, err)
	err = pkiVerifier.Verify([]byte("some-message"), signature)
	assert.NoError(t, err)

	// Test vault signer error handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		genericVaultWriteError: true,
		keyType:                "ecdsa-p256",
		privateKey:             PrivateECKey,
		publicKey:              PublicECKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)

	// Test invalid signature response handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		invalidSignatureResponse: true,
		keyType:                  "ecdsa-p256",
		privateKey:               PrivateECKey,
		publicKey:                PublicECKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		invalidSignatureResponse: true,
		typeAssertionError:       true,
		keyType:                  "ecdsa-p256",
		privateKey:               PrivateECKey,
		publicKey:                PublicECKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	signature, err = vaultSigner.Sign([]byte("some-message"))
	assert.Nil(t, signature)
	assert.Error(t, err)
}

func TestVaultVerify(t *testing.T) {
	// Generate a signature with existing PKISigner
	pkiSigner, err := artifact.NewPKISigner([]byte(PrivateRSAKey))
	assert.NoError(t, err)
	assert.NotNil(t, pkiSigner)

	signature, err := pkiSigner.Sign([]byte("some-message"))
	assert.NoError(t, err)
	assert.NotNil(t, signature)

	// Check normal verify operation
	vaultSigner := VaultSigner{vaultClient: &fakeVaultClient{
		returnPublicKey: true,
		keyType:         "rsa-2048",
		privateKey:      PrivateRSAKey,
		publicKey:       PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.NoError(t, err)

	// Test vault verify error handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyNotFound: true,
		keyType:     "rsa-2048",
		privateKey:  PrivateRSAKey,
		publicKey:   PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.Error(t, err)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		genericVaultReadError: true,
		keyType:               "rsa-2048",
		privateKey:            PrivateRSAKey,
		publicKey:             PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.Error(t, err)

	// Test Key Version not found
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:         "rsa-2048",
		returnPublicKey: true,
		privateKey:      PrivateRSAKey,
		publicKey:       PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 2,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.Error(t, err)

	// Test invalid response data handling
	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:                  "rsa-2048",
		invalidPublicKeyResponse: true,
		privateKey:               PrivateRSAKey,
		publicKey:                PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.Error(t, err)

	vaultSigner = VaultSigner{vaultClient: &fakeVaultClient{
		keyType:                  "rsa-2048",
		invalidPublicKeyResponse: true,
		typeAssertionError:       true,
		privateKey:               PrivateRSAKey,
		publicKey:                PublicRSAKey,
	},
		mountPath:  "",
		keyName:    "",
		keyVersion: 1,
	}
	err = vaultSigner.Verify([]byte("some-message"), signature)
	assert.Error(t, err)
}

type fakeVaultClient struct {
	keyType                  string
	privateKey               string
	publicKey                string
	keyNotFound              bool
	typeAssertionError       bool
	invalidKeyTypeResponse   bool
	invalidSignatureResponse bool
	invalidPublicKeyResponse bool
	genericVaultReadError    bool
	genericVaultWriteError   bool
	returnPublicKey          bool
}

func (c *fakeVaultClient) Read(path string) (*vault.Secret, error) {
	if c.genericVaultReadError {
		return nil, errors.New("Vault Read error")
	}

	if c.keyNotFound {
		return nil, nil
	}

	if c.invalidKeyTypeResponse {
		var invalidKey map[string]interface{}
		if c.typeAssertionError {
			invalidKey = map[string]interface{}{
				"type": 1,
			}
		} else {
			invalidKey = map[string]interface{}{
				"invalid": c.keyType,
			}
		}
		return &vault.Secret{Data: invalidKey}, nil
	}

	if c.invalidPublicKeyResponse {
		var publicKey map[string]interface{}
		if c.typeAssertionError {
			publicKey = map[string]interface{}{
				"keys": map[string]interface{}{
					"1": map[string]interface{}{
						"public_key": 1,
					},
				},
			}
		} else {
			publicKey = map[string]interface{}{
				"keys": map[string]interface{}{
					"1": map[string]interface{}{
						"invalid": "invalid",
					},
				},
			}
		}
		return &vault.Secret{Data: publicKey}, nil
	}

	if c.returnPublicKey {
		publicKey := map[string]interface{}{
			"keys": map[string]interface{}{
				"1": map[string]interface{}{
					"public_key": c.publicKey,
				},
			},
		}
		return &vault.Secret{Data: publicKey}, nil
	}

	testdata := map[string]interface{}{
		"type": c.keyType,
	}
	return &vault.Secret{Data: testdata}, nil
}

func (c *fakeVaultClient) Write(path string, data map[string]interface{}) (*vault.Secret, error) {
	if c.genericVaultWriteError {
		return nil, errors.New("Vault Write errror")
	}

	if c.invalidSignatureResponse {
		var responseData map[string]interface{}
		if c.typeAssertionError {
			responseData = map[string]interface{}{
				"signature": 1,
			}
		} else {
			responseData = map[string]interface{}{
				"invalid": "12345",
			}
		}
		return &vault.Secret{
			Data: responseData,
		}, nil
	}

	sm, err := artifact.GetKeyAndSignMethod([]byte(c.privateKey))
	if err != nil {
		return nil, errors.Wrap(err, "error getting key and sign method")
	}
	hash, err := base64.StdEncoding.DecodeString(data["input"].(string))
	if err != nil {
		return nil, errors.Wrap(err, "error decoding message hash")
	}
	switch c.keyType {
	case "rsa-2048":
		sig, err := rsa.SignPKCS1v15(rand.Reader, sm.Key.(*rsa.PrivateKey), crypto.SHA256, hash)
		if err != nil {
			return nil, errors.Wrap(err, "error signing message hash")
		}
		responseData := map[string]interface{}{
			"signature": "vault:v1:" + base64.StdEncoding.EncodeToString(sig),
		}
		return &vault.Secret{
			Data: responseData,
		}, nil

	case "ecdsa-p256":
		privKey := sm.Key.(*ecdsa.PrivateKey)
		sig, err := privKey.Sign(rand.Reader, hash, nil)
		if err != nil {
			return nil, errors.Wrap(err, "error signing message hash")
		}
		var parsedSig struct{ R, S *big.Int }
		if _, err := asn1.Unmarshal(sig, &parsedSig); err != nil {
			return nil, errors.Wrap(err, "signer: failed to parse ECDSA signature")
		}
		marshaledSigBytes, err := artifact.MarshalECDSASignature(parsedSig.R, parsedSig.S)
		if err != nil {
			return nil, err
		}
		responseData := map[string]interface{}{
			"signature": "vault:v1:" + base64.RawURLEncoding.EncodeToString(marshaledSigBytes),
		}
		return &vault.Secret{
			Data: responseData,
		}, nil
	}
	return nil, nil
}
