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
	"encoding/asn1"
	"encoding/base64"
	"hash/crc32"
	"math/big"
	"time"

	"github.com/mendersoftware/mender-artifact/artifact"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/minio/sha256-simd"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewKMSSigner creates a Signer that signs using a key from
// Google Cloud's Key Management Service.
// Release resources by calling Close().
func NewKMSSigner(ctx context.Context, name string) (*KMS, error) {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "signer: error connecting to KMS")
	}

	return &KMS{
		name:       name,
		client:     client,
		rpcTimeout: 60 * time.Second,
	}, nil
}

type KMS struct {
	name       string
	client     googleKMSClient
	rpcTimeout time.Duration
}

func (k *KMS) Sign(message []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), k.rpcTimeout)
	defer cancel()

	// Although we don't need this verify method, we use this to
	// check that the key fits our supported algorithms. When
	// performing the actual signature, there's no way to actually
	// check the key's algorithm.
	sm, err := k.getKMSKeyAndVerifyMethod(ctx)
	if err != nil {
		return nil, err
	}

	h := sha256.Sum256(message)

	digestCRC32C := checksum(h[:])

	result, err := k.client.AsymmetricSign(ctx, &kmspb.AsymmetricSignRequest{
		Name: k.name,
		Digest: &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{
				Sha256: h[:],
			},
		},
		DigestCrc32C: wrapperspb.Int64(digestCRC32C),
	})
	if err != nil {
		return nil, errors.Wrap(err, "signer: error signing image with KMS")
	}
	if !result.VerifiedDigestCrc32C {
		return nil, errors.New("signer: KMS signing request corrupted")
	}
	if checksum(result.Signature) != result.SignatureCrc32C.Value {
		return nil, errors.New("signer: KMS signing response corrupted")
	}

	switch sm.Method.(type) {
	case *artifact.RSA:
		sigBase64 := make([]byte, base64.StdEncoding.EncodedLen(len(result.Signature)))
		base64.StdEncoding.Encode(sigBase64, result.Signature)
		return sigBase64, nil
	case *artifact.ECDSA256:
		// KMS serializes ECDSA keys in ASN1 format. Convert it back into our own format.
		var parsedSig struct{ R, S *big.Int }
		if _, err := asn1.Unmarshal(result.Signature, &parsedSig); err != nil {
			return nil, errors.Wrap(err, "signer: failed to parse ECDSA signature")
		}
		marshaledSigBytes, err := artifact.MarshalECDSASignature(parsedSig.R, parsedSig.S)
		if err != nil {
			return nil, err
		}
		outputSig := make([]byte, base64.StdEncoding.EncodedLen(len(marshaledSigBytes)))
		base64.StdEncoding.Encode(outputSig, marshaledSigBytes)
		return outputSig, nil
	default:
		return nil, errors.New("signer: unsupported algorithm")
	}
}

func (k *KMS) Verify(message, sig []byte) error {
	ctx, cancel := context.WithTimeout(context.TODO(), k.rpcTimeout)
	defer cancel()

	sm, err := k.getKMSKeyAndVerifyMethod(ctx)
	if err != nil {
		return err
	}

	dec := make([]byte, base64.StdEncoding.DecodedLen(len(sig)))
	decLen, err := base64.StdEncoding.Decode(dec, sig)
	if err != nil {
		return errors.Wrap(err, "signer: error decoding signature")
	}

	return sm.Method.Verify(message, dec[:decLen], sm.Key)
}

func (k *KMS) getKMSKeyAndVerifyMethod(ctx context.Context) (*artifact.SigningMethod, error) {
	response, err := k.client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: k.name})
	if err != nil {
		return nil, errors.Wrap(err, "signer: error getting public key from KMS")
	}

	if checksum([]byte(response.Pem)) != response.PemCrc32C.Value {
		return nil, errors.New("signer: KMS verification response corrupted")
	}

	return artifact.GetKeyAndVerifyMethod([]byte(response.Pem))
}

func (k *KMS) Close() error {
	return k.client.Close()
}

func checksum(data []byte) int64 {
	crcTable := crc32.MakeTable(crc32.Castagnoli)
	return int64(crc32.Checksum(data, crcTable))
}

type googleKMSClient interface {
	AsymmetricSign(context.Context, *kmspb.AsymmetricSignRequest,
		...gax.CallOption) (*kmspb.AsymmetricSignResponse, error)
	GetPublicKey(context.Context, *kmspb.GetPublicKeyRequest,
		...gax.CallOption) (*kmspb.PublicKey, error)
	Close() error
}
