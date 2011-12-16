/*
DKeyczar is a simplified wrapper around Go's native cryptography libraries.  It
is modeled after and compatible with Google's Keyczar library
(http://keyczar.org)

Sample usage is:
   reader = NewFileReader("/path/to/keys")
   crypter = NewCrypter(reader)
   crypter.Encrypt(data)

   Decryption, Signing and Verification use the same minimal API.

   Encrypted data and signatures are encoding with a web-safe base64 encoding.

*/
package dkeyczar

import (
	"bytes"
	"encoding/json"
	"log"
)

type keyCzar struct {
	keymeta keyMeta         // metadata for this key
	keys    map[int]keyIDer // maps versions to keys
	primary int             // integer version of the primary key
}

type Encrypter interface {
	// Encrypt returns an encrypted representing the plaintext bytes passed
	Encrypt(plaintext []uint8) string
}

type Crypter interface {
	Encrypter
	// Decrypt returns the plaintext bytes of an encrypted string
	Decrypt(ciphertext string) []uint8
}

type Signer interface {
	Verifier
	// Sign returns a cryptographic signature for the message
	Sign(message []byte) string
}

type Verifier interface {
	// Verify checks the cryptographic signature for a message
	Verify(message []byte, signature string) bool
}

func (kz *keyCzar) Encrypt(plaintext []uint8) string {

	key := kz.keys[kz.primary]

	encryptKey := key.(encryptKey)

	ciphertext := encryptKey.Encrypt(plaintext)
	s := encodeWeb64String(ciphertext)

	return s

}

func (kz *keyCzar) Decrypt(ciphertext string) []uint8 {

	b, _ := decodeWeb64String(ciphertext)

	if b[0] != kzVersion {
		log.Fatal("bad version: ", b[0])
	}

	keyid := b[1:5]

	for _, k := range kz.keys {
		if bytes.Compare(k.KeyID(), keyid) == 0 {
			decryptKey := k.(decryptEncryptKey)
			return decryptKey.Decrypt(b)
		}
	}

	log.Fatal("unknown keyid=", keyid)

	return nil
}

func (kz *keyCzar) Verify(msg []byte, signature string) bool {

	sigB, _ := decodeWeb64String(signature)

	if sigB[0] != kzVersion {
		log.Fatal("bad version: ", sigB[0])
	}

	keyid, sig := sigB[1:5], sigB[5:]

	// FIXME: ugly :( -- change Verifier.Verify() call instead?
	signedbytes := make([]byte, len(msg)+1)
	copy(signedbytes, msg)
	signedbytes[len(msg)] = kzVersion

	for _, k := range kz.keys {
		if bytes.Compare(k.KeyID(), keyid) == 0 {
			verifyKey := k.(verifyKey)
			return verifyKey.Verify(signedbytes, sig)
		}
	}

	log.Fatal("unknown keyid=", keyid)

	return false
}

func (kz *keyCzar) Sign(msg []byte) string {

	key := kz.keys[kz.primary]

	signingKey := key.(signVerifyKey)

	signedbytes := make([]byte, len(msg)+1)
	copy(signedbytes, msg)
	signedbytes[len(msg)] = kzVersion

	signature := signingKey.Sign(signedbytes)

	h := header(key)
	signature = append(h, signature...)

	s := encodeWeb64String(signature)

	return s
}

// NewCrypter returns an object capable of encrypting and decrypting using the key provded by the reader
func NewCrypter(r KeyReader) (Crypter, error) {
	return newKeyCzar(r, kpDECRYPT_AND_ENCRYPT)
}

// NewEncypter returns an object capable of encrypting using the key provded by the reader
func NewEncrypter(r KeyReader) (Encrypter, error) {
	return newKeyCzar(r, kpENCRYPT)
}

// NewVerifier returns an object capable of verifying signatures using the key provded by the reader
func NewVerifier(r KeyReader) (Verifier, error) {
	return newKeyCzar(r, kpVERIFY)
}

// NewSigner returns an object capable of creating and verifying signatures using the key provded by the reader
func NewSigner(r KeyReader) (Signer, error) {
	return newKeyCzar(r, kpSIGN_AND_VERIFY)
}

func newKeyCzar(r KeyReader, purpose keyPurpose) (*keyCzar, error) {

	kz := new(keyCzar)

	s, _ := r.GetMetadata()

	err := json.Unmarshal([]byte(s), &kz.keymeta)

	if err != nil {
		return nil, err
	}

	if !kz.keymeta.Purpose.isValidPurpose(purpose) {
		return nil, UnacceptablePurpose
	}

	kz.primary = -1
	for _, v := range kz.keymeta.Versions {
		if v.Status == ksPRIMARY {
			if kz.primary == -1 {
				kz.primary = v.VersionNumber
			} else {
				return nil, NoPrimaryKeyException // FIXME: technically, "MultiplePrimaryKeyException"
			}
		}
	}

	if kz.primary == -1 {
		return nil, NoPrimaryKeyException
	}

	switch kz.keymeta.Type {
	case ktAES:
		kz.keys = newAesKeys(r, kz.keymeta)
	case ktHMAC_SHA1:
		kz.keys = newHmacKeys(r, kz.keymeta)
	case ktDSA_PRIV:
		kz.keys = newDsaKeys(r, kz.keymeta)
	case ktDSA_PUB:
		kz.keys = newDsaPublicKeys(r, kz.keymeta)
	case ktRSA_PRIV:
		kz.keys = newRsaKeys(r, kz.keymeta)
	case ktRSA_PUB:
		kz.keys = newRsaPublicKeys(r, kz.keymeta)
	default:
		return nil, UnsupportedTypeException
	}

	return kz, nil
}

const kzVersion = uint8(0)
const kzHeaderLength = 5

func header(key keyIDer) []byte {
	b := make([]byte, kzHeaderLength)
	b[0] = kzVersion
	copy(b[1:], key.KeyID())

	return b
}
