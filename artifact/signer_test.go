// Copyright 2017 Northern.tech AS
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

package artifact

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

const (
	PublicRSAKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
XwaUNml5EhW79AdibBXZiZt8fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne
5vbA+63vRCnrc8QuYwIDAQAB
-----END PUBLIC KEY-----`
	PublicRSAKeyError = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
XwaUNml5EhW79AdibBXZiZt8fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne
5vbA+63vRCnrc8QuYwIDAQAC
-----END PUBLIC KEY-----`
	PublicRSAKeyInvalid = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDSTLzZ9hQq3yBB+dMDVbKem6ia
v1J6opg6DICKkQ4M/yhlw32BCGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKc
5vbA+63vRCnrc8QuYwIDAQAC
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
	PublicECDSAKeyError = `-----BEGIN PUBLIC KEY-----
	MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEVhXD5e34NkyKBh/N4ufpP+bjIrHc0WB/
	h9XSvpMkwxNOhlyGMjhjPQ/RBbnhg0nP4w5fDVBOz/lymh9LHuWXYfeAwVweND9u
	4M26g3yreVOnBNaFiSsuUxSGSCdNmAPP
-----END PUBLIC KEY-----`
	PrivateECDSAKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMOJJlcKM0sMwsOezNKeUXm4BiN6+ZPggu87yuZysDgIoAoGCCqGSM49
AwEHoUQDQgAE9iC/hyQO1UQfw0fFj1RjEjwOvPIBsz6Of3ock/gIwmnhnC/7USo3
yOTl4wVLQKA6mFvMV9o8B9yTBNg3mQS0vA==
-----END EC PRIVATE KEY-----`

	PrivateECDSA384 = `-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDCpMN90MAn1M/PlA6Qf/FuFGJRlHir6jDrnZkInL1MrCMrIIExo5An5
2GNIPtO4fzSgBwYFK4EEACKhZANiAARZmOq7QZX3Z+saM+3Dc19xuB/3iINOy06a
3VMj8JyjHqfO97JXkaW4RHYn4Jakh/EjhU4sQpUHpcsp5V7fXCJtjUfZNgbvhgBN
XR+Oq96ygCg3ua2mL/4uiU2vPnX+tAg=
-----END EC PRIVATE KEY-----`
	PublicECDSA384 = `-----BEGIN PUBLIC KEY-----
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEWZjqu0GV92frGjPtw3Nfcbgf94iDTstO
mt1TI/Ccox6nzveyV5GluER2J+CWpIfxI4VOLEKVB6XLKeVe31wibY1H2TYG74YA
TV0fjqvesoAoN7mtpi/+LolNrz51/rQI
-----END PUBLIC KEY-----`

	PublicDSAKey = `-----BEGIN PUBLIC KEY-----
MIIBtzCCASwGByqGSM44BAEwggEfAoGBAKNcqa1Q/0s3W8OW3YlVgD2SvFUAZJv3
N5vnwxUlxIM4VPR94cNxOQE9TrSMI001twcBC4yYM1WBGNcQLhwuA7EAznkjjpQu
LebyUEKZBd2cJMkPpBG5YF+WOJaXMX1JTtuMQLik/vJlfbQjK7DbT640Fve2B++k
Riq6lq2mmpOJAhUA1Xn1uAM0BH6tO2fUKM2e43IjfvsCgYEAlBmxxsXSGwtUJtip
lGgzyGhymqLXOkTf+DC8AczDT0hJxE0iPVT7ZoJvgsyKSOLSJREndeipSSOXyRSt
oRPUlk2RSYYCvXTGzwfxdS1WoyYFvrij/wFlYIbvTQJoB36wTDI7/Tp+/f9iie+5
HWcFL6NGmeS+N8fz0MgiwVkdkWoDgYQAAoGANzjN9AfCzxcAswYvZyDn3bHR9Foa
XbeslVVE29ZP7iJNkVT1JxFWkfA3/gQXn8h0or87wPGu+bX4jw6BK46mP717RgCT
0dlFBsy2xqtcPzkiW6Sx4pqjYUQC37TJ63/vvXkPlvFUpUzmGzZ9V5mQLupwtQ2z
MIXnMqXyHgMqtVA=
-----END PUBLIC KEY-----`
	PrivateDSAKey = `-----BEGIN DSA PRIVATE KEY-----
MIIBuwIBAAKBgQCjXKmtUP9LN1vDlt2JVYA9krxVAGSb9zeb58MVJcSDOFT0feHD
cTkBPU60jCNNNbcHAQuMmDNVgRjXEC4cLgOxAM55I46ULi3m8lBCmQXdnCTJD6QR
uWBfljiWlzF9SU7bjEC4pP7yZX20Iyuw20+uNBb3tgfvpEYqupatppqTiQIVANV5
9bgDNAR+rTtn1CjNnuNyI377AoGBAJQZscbF0hsLVCbYqZRoM8hocpqi1zpE3/gw
vAHMw09IScRNIj1U+2aCb4LMikji0iURJ3XoqUkjl8kUraET1JZNkUmGAr10xs8H
8XUtVqMmBb64o/8BZWCG700CaAd+sEwyO/06fv3/YonvuR1nBS+jRpnkvjfH89DI
IsFZHZFqAoGANzjN9AfCzxcAswYvZyDn3bHR9FoaXbeslVVE29ZP7iJNkVT1JxFW
kfA3/gQXn8h0or87wPGu+bX4jw6BK46mP717RgCT0dlFBsy2xqtcPzkiW6Sx4pqj
YUQC37TJ63/vvXkPlvFUpUzmGzZ9V5mQLupwtQ2zMIXnMqXyHgMqtVACFFTsC83p
BlUY/oCrAGUGN10F49+c
-----END DSA PRIVATE KEY-----`
)

func TestPublicKey(t *testing.T) {
	m, err := getKeyAndVerifyMethod([]byte(PublicRSAKey))
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.IsType(t, &RSA{}, m.method)
	assert.IsType(t, &rsa.PublicKey{}, m.key)

	m, err = getKeyAndVerifyMethod([]byte(PublicECDSAKey))
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.IsType(t, &ECDSA256{}, m.method)
	assert.IsType(t, &ecdsa.PublicKey{}, m.key)

	m, err = getKeyAndVerifyMethod([]byte(PublicDSAKey))
	assert.Error(t, err)
	assert.Nil(t, m)

	m, err = getKeyAndVerifyMethod([]byte("some ivalid key"))
	assert.Error(t, err)
	assert.Nil(t, m)
	m, err = getKeyAndVerifyMethod([]byte(PublicRSAKeyInvalid))
	assert.Error(t, err)
	assert.Nil(t, m)
}

func TestPrivateKey(t *testing.T) {
	m, err := getKeyAndSignMethod([]byte(PrivateRSAKey))
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.IsType(t, &RSA{}, m.method)
	assert.IsType(t, &rsa.PrivateKey{}, m.key)

	m, err = getKeyAndSignMethod([]byte(PrivateECDSAKey))
	assert.NoError(t, err)
	assert.NotNil(t, m)
	assert.IsType(t, &ECDSA256{}, m.method)
	assert.IsType(t, &ecdsa.PrivateKey{}, m.key)

	m, err = getKeyAndSignMethod([]byte(PrivateDSAKey))
	assert.Error(t, err)
	assert.Nil(t, m)

	m, err = getKeyAndSignMethod([]byte("invalid key"))
	assert.Error(t, err)
	assert.Nil(t, m)
}

func TestRSA(t *testing.T) {
	msg := []byte("this is secret message")

	s := NewSigner([]byte(PrivateRSAKey))
	sig, err := s.Sign(msg)
	assert.NoError(t, err)
	assert.NotNil(t, sig)

	v := NewVerifier([]byte(PublicRSAKey))
	err = v.Verify(msg, sig)
	assert.NoError(t, err)

	// use invalid key
	v = NewVerifier([]byte(PublicRSAKeyError))
	err = v.Verify(msg, sig)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "verification error")
}

func TestRSARaw(t *testing.T) {
	r := RSA{}
	sig, err := r.Sign([]byte("my message"), PublicRSAKey)
	assert.Error(t, err)
	assert.Nil(t, sig)

	err = r.Verify([]byte("my message"), []byte("signature"), PrivateRSAKey)
	assert.Error(t, err)
}

func TestECDSA(t *testing.T) {
	msg := []byte("this is secret message")

	s := NewSigner([]byte(PrivateECDSAKey))
	sig, err := s.Sign(msg)
	assert.NoError(t, err)
	assert.NotNil(t, sig)

	v := NewVerifier([]byte(PublicECDSAKey))
	err = v.Verify(msg, sig)
	assert.NoError(t, err)

	// use invalid key
	v = NewVerifier([]byte(PublicECDSAKeyError))
	err = v.Verify(msg, sig)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "verification failed")

	// use invalid signature
	v = NewVerifier([]byte(PublicECDSAKey))
	// change the first byte of the signature
	sig, err = s.Sign([]byte("this is a different message"))
	assert.NoError(t, err)

	err = v.Verify(msg, sig)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "verification failed")

	// use broken key
	v = NewVerifier([]byte("broken key"))
	err = v.Verify(msg, sig)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "failed to parse")
}

func TestECDSARaw(t *testing.T) {
	r := ECDSA256{}
	// use public key for signign
	sig, err := r.Sign([]byte("my message"), PublicECDSAKey)
	assert.Error(t, err)
	assert.Nil(t, sig)

	// invalid key length
	crypt, err := getKeyAndSignMethod([]byte(PrivateECDSA384))
	assert.NoError(t, err)
	sig, err = r.Sign([]byte("my message"), crypt.key)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "invalid ecdsa curve size")
	assert.Nil(t, sig)

	// use private key for verification
	err = r.Verify([]byte("my message"), []byte("signature"), PrivateECDSAKey)
	assert.Error(t, err)

	crypt, err = getKeyAndVerifyMethod([]byte(PublicECDSAKey))
	assert.NoError(t, err)

	// use wrong size key for verification
	err = r.Verify([]byte("my message"), []byte("signature"), crypt.key)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "invalid ecdsa key size")
}
