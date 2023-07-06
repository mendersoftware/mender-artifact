// Copyright 2023 Northern.tech AS
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

package gcp

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"hash/crc32"
	"testing"
	"time"

	"cloud.google.com/go/kms/apiv1/kmspb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	rsaKeyName           = "test/key/rsa"
	ecdsaKeyName         = "test/key/ecdsa"
	pubKeyPEMHeader      = "PUBLIC KEY"
	ecdsaPubKeyPEMHeader = ""

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

var (
	availableKMSKeys = map[string]testSigningKey{ // key name -> keypair
		rsaKeyName: {
			private: PrivateRSAKey,
			public:  PublicRSAKey,
		},
		ecdsaKeyName: {
			private: PrivateECDSAKey,
			public:  PublicECDSAKey,
		},
	}
)

func TestKMSSignAndVerify(t *testing.T) {
	tests := map[string]*signAndVerifyTestCase{
		"rsa": {
			keyName: rsaKeyName,
		},
		"ecdsa": {
			keyName: ecdsaKeyName,
		},
		"wrong key name": {
			keyName:     "invalid key name",
			wantSignErr: true,
		},
		"corrupted signature rsa": {
			signClient:  &fakeKMSClient{corruptSigningCRC: true},
			keyName:     rsaKeyName,
			wantSignErr: true,
		},
		"corrupted signature ecdsa": {
			signClient:  &fakeKMSClient{corruptSigningCRC: true},
			keyName:     ecdsaKeyName,
			wantSignErr: true,
		},
		"corrupted public key during signing rsa": {
			signClient:  &fakeKMSClient{corruptPublicKeyCRC: true},
			keyName:     rsaKeyName,
			wantSignErr: true,
		},
		"corrupted public key during signing ecdsa": {
			signClient:  &fakeKMSClient{corruptPublicKeyCRC: true},
			keyName:     ecdsaKeyName,
			wantSignErr: true,
		},
		"corrupted public key during verification rsa": {
			verifyClient:  &fakeKMSClient{corruptPublicKeyCRC: true},
			keyName:       rsaKeyName,
			wantVerifyErr: true,
		},
		"corrupted public key during verification ecdsa": {
			verifyClient:  &fakeKMSClient{corruptPublicKeyCRC: true},
			keyName:       ecdsaKeyName,
			wantVerifyErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			msg := []byte("some msg")

			// If either client is nil, set it to a default one.
			if test.signClient == nil {
				test.signClient = &fakeKMSClient{}
			}
			if test.verifyClient == nil {
				test.verifyClient = &fakeKMSClient{}
			}
			kmsSigner := &KMS{
				name:       test.keyName,
				client:     test.signClient,
				rpcTimeout: 60 * time.Second,
			}
			kmsVerifier := &KMS{
				name:       test.keyName,
				client:     test.verifyClient,
				rpcTimeout: 60 * time.Second,
			}

			// Start by signing.
			sig, err := kmsSigner.Sign(msg)
			if err == nil && test.wantSignErr {
				t.Errorf("Sign: got nil error, want an error")
				return
			}
			if err != nil && !test.wantSignErr {
				t.Errorf("Sign: %v", err)
				return
			}

			// If we expected an error during signing, skip the verification.
			if test.wantSignErr {
				return
			}

			// Make sure verification works with the given signature.
			err = kmsVerifier.Verify(msg, sig)
			if err == nil && test.wantVerifyErr {
				t.Errorf("Verify: got nil error, want an error")
				return
			}
			if err != nil && !test.wantVerifyErr {
				t.Errorf("Verify: %v", err)
			}
		})
	}
}

func TestKMSSignatureCompatibility(t *testing.T) {
	tests := []string{rsaKeyName, ecdsaKeyName}
	for _, keyName := range tests {
		t.Run(keyName, func(t *testing.T) {
			msg := []byte("some msg")
			kmsSigner := &KMS{
				name:       keyName,
				client:     &fakeKMSClient{},
				rpcTimeout: 60 * time.Second,
			}
			goldenSigner, err := artifact.NewPKISigner([]byte(availableKMSKeys[keyName].private))
			if err != nil {
				t.Errorf("NewPKISigner: %v", err)
				return
			}

			// Sign with Google KMS, verify with golden verifier.
			sig, err := kmsSigner.Sign(msg)
			if err != nil {
				t.Errorf("Sign: %v", err)
				return
			}
			if err := goldenSigner.Verify(msg, sig); err != nil {
				t.Errorf("Golden Verify: %v", err)
				return
			}

			// Sign with golden signer, verify with Google KMS.
			goldenSig, err := goldenSigner.Sign(msg)
			if err != nil {
				t.Errorf("Golden Sign: %v", err)
				return
			}
			if err := kmsSigner.Verify(msg, goldenSig); err != nil {
				t.Errorf("Verify: %v", err)
			}
		})
	}
}

type signAndVerifyTestCase struct {
	// Optionally specify a client for signing.
	// If nil, a default client will be used.
	signClient *fakeKMSClient
	// Optionally specify a client for verification.
	// If nil, a default client will be used.
	verifyClient  *fakeKMSClient
	keyName       string
	wantSignErr   bool
	wantVerifyErr bool
}

type fakeKMSClient struct {
	corruptSigningCRC   bool
	corruptPublicKeyCRC bool
}

func (f *fakeKMSClient) AsymmetricSign(_ context.Context, req *kmspb.AsymmetricSignRequest, _ ...gax.CallOption) (*kmspb.AsymmetricSignResponse, error) {
	key, err := f.findKey(req.Name)
	if err != nil {
		return nil, err
	}
	sm, err := artifact.GetKeyAndSignMethod([]byte(key.private))
	if err != nil {
		return nil, fmt.Errorf("key %q: %v", req.Name, err)
	}

	crcTable := crc32.MakeTable(crc32.Castagnoli)
	digestCRC32C := crc32.Checksum(req.Digest.GetSha256(), crcTable)
	verifiedDigestCRC32C := int64(digestCRC32C) == req.DigestCrc32C.Value

	// We can't reuse sm.Method.sign because those functions will hash the data
	// an additional time. We just want the signature, since we only have the
	// hash available in this function.
	var sig []byte
	switch sm.Method.(type) {
	case *artifact.RSA:
		sig, err = rsa.SignPKCS1v15(rand.Reader, sm.Key.(*rsa.PrivateKey), crypto.SHA256, req.Digest.GetSha256())
		if err != nil {
			return nil, fmt.Errorf("key %q: %v", req.Name, err)
		}
	case *artifact.ECDSA256:
		privKey := sm.Key.(*ecdsa.PrivateKey)
		sig, err = privKey.Sign(rand.Reader, req.Digest.GetSha256(), nil)
		if err != nil {
			return nil, fmt.Errorf("key %q: %v", req.Name, err)
		}
	default:
		return nil, fmt.Errorf("key %q: unsupported signing algorithm", req.Name)
	}

	sigCRC32C := crc32.Checksum(sig, crcTable)
	if f.corruptSigningCRC {
		sigCRC32C = 123456
	}

	return &kmspb.AsymmetricSignResponse{
		Signature:            sig,
		VerifiedDigestCrc32C: verifiedDigestCRC32C,
		SignatureCrc32C:      wrapperspb.Int64(int64(sigCRC32C)),
	}, nil
}

func (f *fakeKMSClient) GetPublicKey(_ context.Context, req *kmspb.GetPublicKeyRequest, _ ...gax.CallOption) (*kmspb.PublicKey, error) {
	key, err := f.findKey(req.Name)
	if err != nil {
		return nil, err
	}

	crcTable := crc32.MakeTable(crc32.Castagnoli)
	pemCRC32C := crc32.Checksum([]byte(key.public), crcTable)
	if f.corruptPublicKeyCRC {
		pemCRC32C = 123456
	}
	return &kmspb.PublicKey{
		Pem:       key.public,
		PemCrc32C: wrapperspb.Int64(int64(pemCRC32C)),
	}, nil
}

func (f *fakeKMSClient) findKey(name string) (*testSigningKey, error) {
	if name == "" {
		return nil, errors.New("missing Name field")
	}
	key, keyFound := availableKMSKeys[name]
	if !keyFound {
		return nil, fmt.Errorf("key %q not found", name)
	}
	return &key, nil
}

func (f *fakeKMSClient) Close() error {
	return nil
}

type testSigningKey struct {
	private, public string
}
