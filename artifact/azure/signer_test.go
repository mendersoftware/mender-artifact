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

package azure

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/mendersoftware/mender-artifact/artifact"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/go-jose/go-jose/v3/json"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/stretchr/testify/assert"
)

const (
	PublicRSAKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
XwaUNml5EhW79AdibBXZiZt8fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne
5vbA+63vRCnrc8QuYwIDAQAB
-----END PUBLIC KEY-----`
	PrivateRSAKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQDSTLzZ9hQq3yBB+dMDVbKem6iav1J6opg6DICKkQ4M/yhlw32B
CGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKcXwaUNml5EhW79AdibBXZiZt8
fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne5vbA+63vRCnrc8QuYwIDAQAB
AoGAQKIRELQOsrZsxZowfj/ia9jPUvAmO0apnn2lK/E07k2lbtFMS1H4m1XtGr8F
oxQU7rLyyP/FmeJUqJyRXLwsJzma13OpxkQtZmRpL9jEwevnunHYJfceVapQOJ7/
6Oz0pPWEq39GCn+tTMtgSmkEaSH8Ki9t32g9KuQIKBB2hbECQQDsg7D5fHQB1BXG
HJm9JmYYX0Yk6Z2SWBr4mLO0C4hHBnV5qPCLyevInmaCV2cOjDZ5Sz6iF5RK5mw7
qzvFa8ePAkEA46Anom3cNXO5pjfDmn2CoqUvMeyrJUFL5aU6W1S6iFprZ/YwdHcC
kS5yTngwVOmcnT65Vnycygn+tZan2A0h7QJBAJNlowZovDdjgEpeCqXp51irD6Dz
gsLwa6agK+Y6Ba0V5mJyma7UoT//D62NYOmdElnXPepwvXdMUQmCtpZbjBsCQD5H
VHDJlCV/yzyiJz9+tZ5giaAkO9NOoUBsy6GvdfXWn2prXmiPI0GrrpSvp7Gj1Tjk
r3rtT0ysHWd7l+Kx/SUCQGlitd5RDfdHl+gKrCwhNnRG7FzRLv5YOQV81+kh7SkU
73TXPIqLESVrqWKDfLwfsfEpV248MSRou+y0O1mtFpo=
-----END RSA PRIVATE KEY-----`

	// openssl ecparam -genkey -name secp256r1 -out key.pem
	// openssl ec -in key.pem -pubout
	PublicECDSAKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE9iC/hyQO1UQfw0fFj1RjEjwOvPIB
sz6Of3ock/gIwmnhnC/7USo3yOTl4wVLQKA6mFvMV9o8B9yTBNg3mQS0vA==
-----END PUBLIC KEY-----`
	PrivateECDSAKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMOJJlcKM0sMwsOezNKeUXm4BiN6+ZPggu87yuZysDgIoAoGCCqGSM49
AwEHoUQDQgAE9iC/hyQO1UQfw0fFj1RjEjwOvPIBsz6Of3ock/gIwmnhnC/7USo3
yOTl4wVLQKA6mFvMV9o8B9yTBNg3mQS0vA==
-----END EC PRIVATE KEY-----`
)

type testKey struct {
	public  string
	private string
}

var keys map[string]testKey = map[string]testKey{
	"rsa-test-key": {
		public:  PublicRSAKey,
		private: PrivateRSAKey,
	},
	"ec-test-key": {
		public:  PublicECDSAKey,
		private: PrivateECDSAKey,
	},
}

type fakeAzureClient struct{}

var invalidNames = []string{
	"-name",
	"name-",
	"invalid--name",
	"invalid_name",
	"42name",
	"name*://test",
	"na",
	"nameneedstobelessthan25chars",
}

func TestAzureSigner(t *testing.T) {
	t.Setenv("KEY_VAULT_NAME", "test-keyvault")
	signer, err := NewKeyVaultSigner("test-key")
	assert.NoError(t, err)
	assert.NotNil(t, signer)
	assert.IsType(t, &azureKeyVault{}, signer)

	// Test empty key vault name
	t.Setenv("KEY_VAULT_NAME", "")
	signer, err = NewKeyVaultSigner("test-key")
	assert.Error(t, err)
	assert.Nil(t, signer)

	// Test invalid key vault name
	for _, v := range invalidNames {
		assert.False(t, validateName(v))
	}
}

func TestAzureRSASignAndVerify(t *testing.T) {
	azureSigner := azureKeyVault{
		keyName: "rsa-test-key",
		client:  &fakeAzureClient{},
	}
	msg := "Test message"
	sig, err := azureSigner.Sign([]byte(msg))
	assert.NoError(t, err)
	assert.NotNil(t, sig)

	// Verify valid signature
	err = azureSigner.Verify([]byte(msg), sig)
	assert.NoError(t, err)

	// Test invalid signature
	buf := make([]byte, 256)
	rand.Read(buf)
	sig = make([]byte, base64.StdEncoding.EncodedLen(len(buf)))
	base64.StdEncoding.Encode(sig, buf)
	err = azureSigner.Verify([]byte(msg), sig)
	assert.Error(t, err)
}

func TestAzureECDSASignAndVerify(t *testing.T) {
	azureSigner := azureKeyVault{
		keyName: "ec-test-key",
		client:  &fakeAzureClient{},
	}
	msg := "Some message"
	sig, err := azureSigner.Sign([]byte(msg))
	assert.NoError(t, err)
	assert.NotNil(t, sig)

	// Verify valid signature
	err = azureSigner.Verify([]byte(msg), sig)
	assert.NoError(t, err)

	// Test invalid signature
	buf := make([]byte, 72)
	rand.Read(buf)
	sig = make([]byte, base64.StdEncoding.EncodedLen(len(buf)))
	base64.StdEncoding.Encode(sig, buf)
	err = azureSigner.Verify([]byte(msg), sig)
	assert.Error(t, err)
}

func (c *fakeAzureClient) GetKey(ctx context.Context, name string, version string,
	options *azkeys.GetKeyOptions) (azkeys.GetKeyResponse, error) {
	testKey, found := keys[name]
	if !found {
		return azkeys.GetKeyResponse{}, fmt.Errorf("invalid key name")
	}
	block, _ := pem.Decode([]byte(testKey.public))
	if block == nil {
		return azkeys.GetKeyResponse{}, fmt.Errorf("error decoding public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return azkeys.GetKeyResponse{}, fmt.Errorf("failed to parse public key")
	}

	key, err := jwk.New(pub)
	if err != nil {
		return azkeys.GetKeyResponse{}, fmt.Errorf("error creating jwk.Key")
	}
	jwk.AssignKeyID(key)
	buf, err := json.Marshal(key)
	if err != nil {
		return azkeys.GetKeyResponse{}, fmt.Errorf("error marshalling key")
	}
	var azkey azkeys.JSONWebKey
	if err := azkey.UnmarshalJSON(buf); err != nil {
		return azkeys.GetKeyResponse{}, fmt.Errorf("error unmarshalling JSON into JSONWebKey")
	}
	return azkeys.GetKeyResponse{
		azkeys.KeyBundle{
			Key: &azkey,
		},
	}, nil
}

func (c *fakeAzureClient) Sign(ctx context.Context, name string, version string,
	parameters azkeys.SignParameters, options *azkeys.SignOptions) (azkeys.SignResponse, error) {
	testKey, found := keys[name]
	if !found {
		return azkeys.SignResponse{}, fmt.Errorf("invalid key name")
	}
	sm, err := artifact.GetKeyAndSignMethod([]byte(testKey.private))
	if err != nil {
		return azkeys.SignResponse{}, fmt.Errorf("key %s: %v", name, err)
	}

	var sig []byte
	switch sm.Method.(type) {
	case *artifact.RSA:
		if *parameters.Algorithm != azkeys.SignatureAlgorithmRS256 {
			return azkeys.SignResponse{}, fmt.Errorf("error: key (RSA) - algorithm (%s) mismatch",
				*parameters.Algorithm)
		}
		sig, err = rsa.SignPKCS1v15(nil, sm.Key.(*rsa.PrivateKey), crypto.SHA256, parameters.Value)
		if err != nil {
			return azkeys.SignResponse{}, fmt.Errorf("key %s: %v", name, err)
		}
	case *artifact.ECDSA256:
		if *parameters.Algorithm != azkeys.SignatureAlgorithmES256 {
			return azkeys.SignResponse{}, fmt.Errorf("error: key (ECDSA) - algorithm (%s) mismatch",
				*parameters.Algorithm)
		}
		privKey := sm.Key.(*ecdsa.PrivateKey)
		sig, err = privKey.Sign(rand.Reader, parameters.Value, nil)
		if err != nil {
			return azkeys.SignResponse{}, fmt.Errorf("key %s: %v", name, err)
		}
	default:
		return azkeys.SignResponse{}, fmt.Errorf("key %s: unsupported signing algorithm", name)
	}
	return azkeys.SignResponse{
		azkeys.KeyOperationResult{
			Result: sig,
		},
	}, nil
}
