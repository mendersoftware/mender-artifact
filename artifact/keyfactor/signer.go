/*
Copyright 2023 The Keyfactor Command Authors.

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
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/mendersoftware/mender-artifact/artifact"

	"github.com/Keyfactor/signserver-go-client-sdk/api/signserver"
	"github.com/minio/sha256-simd"
)

type SignServerSigner struct {
	client     *signserver.APIClient
	workerName string
}

func NewSignServerSigner(workerName string) (*SignServerSigner, error) {
	if workerName == "" {
		return nil, errors.New("workerName must be set")
	}

	// Create SignServer API Client
	config := signserver.NewConfiguration()

	// Ensure that configuration picked up from environment variables
	if config.Host == "" {
		return nil, errors.New("SignServer Hostname must be set via environment variable " +
			"SIGNSERVER_HOSTNAME")
	}
	if config.ClientCertificatePath == "" {
		return nil, errors.New("SignServer Client Certificate Path must be set via " +
			"environment variable SIGNSERVER_CLIENT_CERT_PATH")
	}
	// SignServer CA Certificate Path is optional
	// SignServer Key Path is optional - It could be in the
	// certificate pointed to by ClientCertificatePath

	// Create SignServer API Client
	client, err := signserver.NewAPIClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating SignServer API client: %s", err.Error())
	}

	signserverSigner := &SignServerSigner{
		client:     client,
		workerName: workerName,
	}

	return signserverSigner, nil
}

func (s *SignServerSigner) Sign(message []byte) ([]byte, error) {
	// Use the internal sign method to sign the message
	signature, _, err := s.sign(message)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// sign signs the given message using the configured worker,
// and returns the signature and the signer's certificate.
func (s *SignServerSigner) sign(message []byte) ([]byte, *x509.Certificate, error) {
	if s.workerName == "" {
		return nil, nil, errors.New("workerName must be set")
	}

	request := signserver.ProcessRequest{}

	// Calculate SHA-256 digest of message
	hash := sha256.Sum256(message)

	// Base64 encode the digest
	request.SetData(base64.StdEncoding.EncodeToString(hash[:]))
	request.SetEncoding("BASE64")

	// Communicate to SignServer that the digest is already hashed,
	// and that the hash algorithm is SHA-256
	// See https://doc.primekey.com/signserver/signserver-reference/client-side-hashing
	request.SetMetaData(map[string]string{
		"USING_CLIENTSUPPLIED_HASH":      "true",
		"CLIENTSIDE_HASHDIGESTALGORITHM": "SHA-256",
	})

	// Use the configured worker to sign the digest
	// This request uses the POST /workers/{idOrName}/process endpoint
	// See https://doc.primekey.com/signserver/signserver-integration/rest-interface
	signatureProps, _, err := s.client.WorkersAPI.
		Sign(context.Background(), s.workerName).
		ProcessRequest(request).
		Execute()
	if err != nil {
		detail := fmt.Sprintf("failed to sign message with worker "+
			"called %s", s.workerName)

		var bodyError *signserver.GenericOpenAPIError
		ok := errors.As(err, &bodyError)
		if ok {
			detail += fmt.Sprintf(" - %s", string(bodyError.Body()))
		}

		return nil, nil, errors.New(detail)
	}

	// SignServer returns the signer's certificate (public key) in Base64-encoded
	// DER (PEM without header/footer)
	// Decode the Base64 encoded DER
	der, err := base64.StdEncoding.DecodeString(signatureProps.GetSignerCertificate())
	if err != nil {
		return nil, nil, err
	}

	// Parse the DER into a certificate object
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}

	// The signature is also returned in Base64 encoded DER
	// in the format of the signature algorithm configured
	// on the worker. For example, if the worker's algorithm
	// is configured as NONEwithRSA, then the signature algorithm
	// will be RSASSA-PKCS1_v1.5 (PKCS#1 v1.5 signature with RSA)

	// See https://doc.primekey.com/signserver/
	// signserver-reference/signserver-workers/
	// signserver-signers/plain-signer/plain-signer-algorithm-support
	signature := []byte(signatureProps.GetData())

	// Return the signature and the signer's certificate
	return signature, certificate, nil
}

func (s *SignServerSigner) Verify(message, sig []byte) error {
	// Get public key from SignServer
	keyPem, err := s.getPublicKey()
	if err != nil {
		return err
	}

	// Retrieve the appropriate verification method from the key in the PEM block
	method, err := artifact.GetKeyAndVerifyMethod(keyPem)
	if err != nil {
		return err
	}

	// Decode the signature from Base64
	dec := make([]byte, base64.StdEncoding.DecodedLen(len(sig)))
	decLen, err := base64.StdEncoding.Decode(dec, sig)
	if err != nil {
		return fmt.Errorf("signer: error decoding signature: %s", err.Error())
	}

	// Verify the signature
	return method.Method.Verify(message, dec[:decLen], method.Key)
}

// getPublicKey returns the public key from the configured worker
// by signing a dummy message and extracting the
// signer's certificate. Public key is returned in PEM format.
func (s *SignServerSigner) getPublicKey() ([]byte, error) {
	// Sign a dummy message
	_, certificate, err := s.sign([]byte("dummy"))
	if err != nil {
		return nil, fmt.Errorf("failed to get public key from worker called %q: %s",
			s.workerName, err.Error())
	}

	// Build the appropriate PEM block containing marshalled DER bytes and the appropriate header
	var pemBlock *pem.Block
	switch certificate.PublicKeyAlgorithm {
	case x509.RSA:
		pubKey, ok := certificate.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("failed to get public key from worker called %q: %s",
				s.workerName, "failed to parse RSA public key")
		}
		derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal RSA public key to DER: %v", err)
		}
		pemBlock = &pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: derBytes,
		}
	case x509.ECDSA:
		pubKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("failed to get public key from worker called %q: %s",
				s.workerName, "failed to parse ECDSA public key")
		}
		derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal ECDSA public key to DER: %v", err)
		}
		pemBlock = &pem.Block{
			Type:  "ECDSA PUBLIC KEY",
			Bytes: derBytes,
		}

	case x509.Ed25519:
		pubKey, ok := certificate.PublicKey.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("failed to get public key from worker called %q: %s",
				s.workerName, "failed to parse Ed25519 public key")
		}
		derBytes, err := x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Ed25519 public key to DER: %v", err)
		}
		pemBlock = &pem.Block{
			Type:  "ED25519 PUBLIC KEY",
			Bytes: derBytes,
		}
	default:
		return nil, fmt.Errorf("unknown key type in certificate: %s",
			certificate.PublicKeyAlgorithm.String())
	}

	// Encode the PEM block to PEM format
	return pem.EncodeToMemory(pemBlock), nil
}
