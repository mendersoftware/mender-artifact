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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"

	"github.com/mendersoftware/mender-artifact/artifact"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/pkg/errors"
)

type azureClient interface {
	GetKey(ctx context.Context, name string, version string,
		options *azkeys.GetKeyOptions) (azkeys.GetKeyResponse, error)
	Sign(ctx context.Context, name string, version string, parameters azkeys.SignParameters,
		options *azkeys.SignOptions) (azkeys.SignResponse, error)
}

type azureKeyVault struct {
	keyName    string
	keyVersion string
	client     azureClient
}

func NewKeyVaultSigner(keyName string) (*azureKeyVault, error) {
	keyVaultName := os.Getenv("KEY_VAULT_NAME")
	if keyVaultName == "" {
		return nil, fmt.Errorf("azure: no key vault name specified")
	}
	if !validateName(keyVaultName) {
		return nil, fmt.Errorf("azure: invalid key vault name: %s", keyVaultName)
	}
	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	// Key version is optional. If not set, it will use the latest key
	keyVersion := os.Getenv("KEY_VAULT_KEY_VERSION")
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, errors.Wrap(err, "azure: failed to obtain credentials: %v")
	}
	client, err := azkeys.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		return nil, errors.Wrap(err, "azure: failed to create client")
	}

	return &azureKeyVault{
		keyName:    keyName,
		keyVersion: keyVersion,
		client:     client,
	}, nil
}

func validateName(name string) bool {
	r := regexp.MustCompile("^[a-zA-Z](-?[a-zA-Z0-9]+)*$")
	return r.MatchString(name) && len(name) >= 3 && len(name) <= 24
}

func (k *azureKeyVault) Sign(message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	keyType, err := k.getKeyType()
	if err != nil {
		return nil, err
	}

	var algorithm azkeys.SignatureAlgorithm
	switch *keyType {
	case azkeys.KeyTypeEC:
		algorithm = azkeys.SignatureAlgorithmES256
	case azkeys.KeyTypeRSA:
		algorithm = azkeys.SignatureAlgorithmRS256
	default:
		return nil, fmt.Errorf("azure: unsupported key type %s", *keyType)
	}
	resp, err := k.client.Sign(context.TODO(), k.keyName, k.keyVersion, azkeys.SignParameters{
		Algorithm: &algorithm,
		Value:     hash[:],
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "azure: failed to sign message")
	}
	sig := make([]byte, base64.StdEncoding.EncodedLen(len(resp.Result)))
	base64.StdEncoding.Encode(sig, resp.Result)
	return sig, nil
}

func (k *azureKeyVault) Verify(message, sig []byte) error {
	resp, err := k.client.GetKey(context.TODO(), k.keyName, k.keyVersion, nil)
	if err != nil {
		return errors.Wrap(err, "azure: failed to get key")
	}

	dec := make([]byte, base64.StdEncoding.DecodedLen(len(sig)))
	decLen, err := base64.StdEncoding.Decode(dec, sig)
	if err != nil {
		return errors.Wrap(err, "azure: error decoding signature")
	}
	buf, err := resp.Key.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "azure: error marshalling JSONWebKey")
	}
	key, err := jwk.ParseKey(buf)
	if err != nil {
		return errors.Wrap(err, "azure: error parsing JSON Web key")
	}
	keyPem, err := jwk.Pem(key)
	if err != nil {
		return errors.Wrap(err, "azure: error converting to PEM")
	}
	sm, err := artifact.GetKeyAndVerifyMethod(keyPem)
	if err != nil {
		return err
	}
	return sm.Method.Verify(message, dec[:decLen], sm.Key)
}

func (k *azureKeyVault) getKeyType() (*azkeys.KeyType, error) {
	resp, err := k.client.GetKey(context.TODO(), k.keyName, k.keyVersion, nil)
	if err != nil {
		return nil, errors.Wrap(err, "azure: failed to get key")
	}
	return resp.Key.Kty, nil
}
