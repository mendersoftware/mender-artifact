// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
// Code generated by Microsoft (R) Go Code Generator. DO NOT EDIT.

package azkeys

// BackupKeyResponse contains the response from method Client.BackupKey.
type BackupKeyResponse struct {
	// The backup key result, containing the backup blob.
	BackupKeyResult
}

// CreateKeyResponse contains the response from method Client.CreateKey.
type CreateKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// DecryptResponse contains the response from method Client.Decrypt.
type DecryptResponse struct {
	// The key operation result.
	KeyOperationResult
}

// DeleteKeyResponse contains the response from method Client.DeleteKey.
type DeleteKeyResponse struct {
	// A DeletedKey consisting of a WebKey plus its Attributes and deletion info
	DeletedKey
}

// EncryptResponse contains the response from method Client.Encrypt.
type EncryptResponse struct {
	// The key operation result.
	KeyOperationResult
}

// GetDeletedKeyResponse contains the response from method Client.GetDeletedKey.
type GetDeletedKeyResponse struct {
	// A DeletedKey consisting of a WebKey plus its Attributes and deletion info
	DeletedKey
}

// GetKeyResponse contains the response from method Client.GetKey.
type GetKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// GetKeyRotationPolicyResponse contains the response from method Client.GetKeyRotationPolicy.
type GetKeyRotationPolicyResponse struct {
	// Management policy for a key.
	KeyRotationPolicy
}

// GetRandomBytesResponse contains the response from method Client.GetRandomBytes.
type GetRandomBytesResponse struct {
	// The get random bytes response object containing the bytes.
	RandomBytes
}

// ImportKeyResponse contains the response from method Client.ImportKey.
type ImportKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// ListDeletedKeyPropertiesResponse contains the response from method Client.NewListDeletedKeyPropertiesPager.
type ListDeletedKeyPropertiesResponse struct {
	// A list of keys that have been deleted in this vault.
	DeletedKeyPropertiesListResult
}

// ListKeyPropertiesResponse contains the response from method Client.NewListKeyPropertiesPager.
type ListKeyPropertiesResponse struct {
	// The key list result.
	KeyPropertiesListResult
}

// ListKeyPropertiesVersionsResponse contains the response from method Client.NewListKeyPropertiesVersionsPager.
type ListKeyPropertiesVersionsResponse struct {
	// The key list result.
	KeyPropertiesListResult
}

// PurgeDeletedKeyResponse contains the response from method Client.PurgeDeletedKey.
type PurgeDeletedKeyResponse struct {
	// placeholder for future response values
}

// RecoverDeletedKeyResponse contains the response from method Client.RecoverDeletedKey.
type RecoverDeletedKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// ReleaseResponse contains the response from method Client.Release.
type ReleaseResponse struct {
	// The release result, containing the released key.
	KeyReleaseResult
}

// RestoreKeyResponse contains the response from method Client.RestoreKey.
type RestoreKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// RotateKeyResponse contains the response from method Client.RotateKey.
type RotateKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// SignResponse contains the response from method Client.Sign.
type SignResponse struct {
	// The key operation result.
	KeyOperationResult
}

// UnwrapKeyResponse contains the response from method Client.UnwrapKey.
type UnwrapKeyResponse struct {
	// The key operation result.
	KeyOperationResult
}

// UpdateKeyResponse contains the response from method Client.UpdateKey.
type UpdateKeyResponse struct {
	// A KeyBundle consisting of a WebKey plus its attributes.
	KeyBundle
}

// UpdateKeyRotationPolicyResponse contains the response from method Client.UpdateKeyRotationPolicy.
type UpdateKeyRotationPolicyResponse struct {
	// Management policy for a key.
	KeyRotationPolicy
}

// VerifyResponse contains the response from method Client.Verify.
type VerifyResponse struct {
	// The key verify result.
	KeyVerifyResult
}

// WrapKeyResponse contains the response from method Client.WrapKey.
type WrapKeyResponse struct {
	// The key operation result.
	KeyOperationResult
}
