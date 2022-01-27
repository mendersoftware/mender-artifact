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
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"

	vault "github.com/hashicorp/vault/api"
	"github.com/minio/sha256-simd"
	"github.com/pkg/errors"
)

type VaultSigner struct {
	vaultClient vaultLogicalClient
	mountPath   string
	keyName     string
	keyVersion  int
}

func NewVaultSigner(vaultKeyName string) (*VaultSigner, error) {
	//Create Vault client
	config := vault.DefaultConfig()
	client, err := vault.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "error creating Hashicorp Vault client")
	}

	if os.Getenv("VAULT_TOKEN") == "" {
		return nil, errors.New("Please provide a Hashicorp Vault token via " +
			"VAULT_TOKEN environment variable.")
	}

	vaultMountPath := os.Getenv("VAULT_MOUNT_PATH")
	if vaultMountPath == "" {
		return nil, errors.New("Please provide the mount path of the used transit engine " +
			"via VAULT_MOUNT_PATH environment variable.")
	}

	// Use key version from environment variable, if set.
	// If not, use default key version (1)
	vaultKeyVersion := 1
	if os.Getenv("VAULT_KEY_VERSION") != "" {
		vaultKeyVersion, err = strconv.Atoi(os.Getenv("VAULT_KEY_VERSION"))
		if err != nil {
			return nil, errors.Wrap(err, "Error converting VAULT_KEY_VERSION to integer")
		}
	}

	return &VaultSigner{
		vaultClient: client.Logical(),
		mountPath:   vaultMountPath,
		keyName:     vaultKeyName,
		keyVersion:  vaultKeyVersion,
	}, nil
}

func (s *VaultSigner) Sign(message []byte) ([]byte, error) {

	// Hash message and convert hash to base64
	hasher := sha256.New()
	hasher.Write(message)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))

	// Get key type from vault (RSA or ECDSA)
	keyType, err := s.getKeyType()
	if err != nil {
		return nil, err
	}

	// Call signing function depending on keyType
	if strings.HasPrefix(keyType, "ecdsa-p256") {
		return s.VaultECDSASign(hash)
	} else if strings.HasPrefix(keyType, "rsa") {
		return s.VaultRSASign(hash)
	}

	return nil, errors.New("unsupported key type: " + keyType)
}

func (s *VaultSigner) Verify(message, sig []byte) error {
	sm, err := s.getVaultKeyAndVerifyMethod()
	if err != nil {
		return errors.Wrap(err, "Error while generating verifier")
	}

	dec := make([]byte, base64.StdEncoding.DecodedLen(len(sig)))
	decLen, err := base64.StdEncoding.Decode(dec, sig)
	if err != nil {
		return errors.Wrap(err, "signer: error decoding signature")
	}

	return sm.Method.Verify(message, dec[:decLen], sm.Key)
}

func (s *VaultSigner) getVaultKeyAndVerifyMethod() (*artifact.SigningMethod, error) {
	// Get public key from Vault
	response, err := s.vaultClient.Read(s.mountPath + "/keys/" + s.keyName)
	if err != nil {
		return nil, errors.Wrap(err, "signer: error getting public "+
			"key from Hashicorp Vault")
	}

	if response == nil {
		return nil, errors.New("error getting public key from Hashicorp Vault: " +
			"Key \"" + s.keyName + "\" not found")
	}
	// Extract public key from response
	keys, exist := response.Data["keys"]
	if !exist {
		return nil, errors.New("No keys found in response")
	}

	keys_map, ok := keys.(map[string]interface{})
	if !ok {
		return nil, errors.New("type assertion error: expected map[string]interface{}")
	}

	selected_key, exist := keys_map[strconv.Itoa(s.keyVersion)]
	if !exist {
		return nil, errors.New("Key version not found: v" + strconv.Itoa(s.keyVersion))
	}

	selected_key_map, ok := selected_key.(map[string]interface{})
	if !ok {
		return nil, errors.New("type assertion error: expected map[string]interface{}")
	}

	public_key, exist := selected_key_map["public_key"]
	if !exist {
		return nil, errors.New("Public key not found in response from Hashicorp Vault")
	}

	public_key_string, ok := public_key.(string)
	if !ok {
		return nil, errors.New("type assertion error: expected public key as string")
	}

	return artifact.GetKeyAndVerifyMethod([]byte(public_key_string))
}

func (s *VaultSigner) getKeyType() (string, error) {
	// Get key type from vault (RSA or ECDSA)
	response, err := s.vaultClient.Read(s.mountPath +
		"/keys/" + s.keyName)
	if err != nil {
		return "", errors.Wrap(err, "error getting key type from Hashicorp Vault")
	}

	if response == nil {
		return "", errors.New("error getting key type from Hashicorp Vault: " +
			"Key \"" + s.keyName + "\" not found")
	}

	key_type, exist := response.Data["type"]
	if !exist {
		return "", errors.New("key type not found in response from Hashicorp Vault")
	}

	key_type_string, ok := key_type.(string)
	if !ok {
		return "", errors.New("type assertion error: expected key type as string")
	}
	return key_type_string, nil
}

func (s *VaultSigner) VaultECDSASign(hash string) ([]byte, error) {
	// Generate string map for signing request
	signing_request := map[string]interface{}{
		"input":                hash,
		"prehashed":            true,
		"marshaling_algorithm": "jws",
		"key_version":          s.keyVersion,
	}

	// Define path for vault client to sign the hash
	path := s.mountPath + "/sign/" + s.keyName + "/sha2-256"

	// Send signing request to Vault server
	response, err := s.vaultClient.Write(path, signing_request)

	// Check request Status Code
	if err != nil {
		return nil, errors.Wrap(err, "error sending signing request to Hashicorp Vault")
	}

	// Extract signature from Vault response
	signature, exist := response.Data["signature"]
	if !exist {
		return nil, errors.New("signature not found in response from Hashicorp Vault")
	}

	signature_string, ok := signature.(string)
	if !ok {
		return nil, errors.New("type assertion error: expected signature as string")
	}

	// Vault signature is URL-safe base64 encoded.
	// We need to re-encode it with standard base64.
	signature_extracted := strings.Split(signature_string, ":")[2]
	signature_raw, err := base64.RawURLEncoding.DecodeString(signature_extracted)

	if err != nil {
		return nil, errors.Wrap(err, "error while decoding signature")
	}

	return []byte(base64.StdEncoding.EncodeToString(signature_raw)), nil
}

func (s *VaultSigner) VaultRSASign(hash string) ([]byte, error) {
	// Generate string map for signing request
	signing_request := map[string]interface{}{
		"input":               hash,
		"prehashed":           true,
		"signature_algorithm": "pkcs1v15",
		"key_version":         s.keyVersion,
	}

	// Define path for vault client to sign the hash
	path := s.mountPath + "/sign/" + s.keyName + "/sha2-256"

	// Send signing request to Vault server
	response, err := s.vaultClient.Write(path, signing_request)

	// Check request status code
	if err != nil {
		return nil, errors.Wrap(err, "error sending signing request to Hashicorp Vault")
	}

	// Extract signature from Vault response
	signature, exist := response.Data["signature"]
	if !exist {
		return nil, errors.New("signature not found in response from Hashicorp Vault")
	}

	signature_string, ok := signature.(string)
	if !ok {
		return nil, errors.New("type assertion error: expected signature as string")
	}

	signature_extracted := strings.Split(signature_string, ":")[2]
	return []byte(signature_extracted), nil
}

type vaultLogicalClient interface {
	Read(path string) (*vault.Secret, error)
	Write(path string, data map[string]interface{}) (*vault.Secret, error)
}
