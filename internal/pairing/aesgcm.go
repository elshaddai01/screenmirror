package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
)

// aeadCipher mirrors AOSP's Aes128Gcm: an AES-128-GCM cipher keyed via
// HKDF-SHA256 over the SPAKE2 shared secret, using separate monotonically
// increasing sequence numbers (as the low 8 bytes of a 12-byte nonce, the
// rest zero) for the encrypt and decrypt directions.
type aeadCipher struct {
	aead   cipher.AEAD
	encSeq uint64
	decSeq uint64
}

func newAeadCipher(keyMaterial []byte) (*aeadCipher, error) {
	key, err := hkdf.Key(sha256.New, keyMaterial, nil, "adb pairing_auth aes-128-gcm key", 16)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &aeadCipher{aead: aead}, nil
}

func (c *aeadCipher) Encrypt(plaintext []byte) []byte {
	nonce := make([]byte, c.aead.NonceSize())
	binary.LittleEndian.PutUint64(nonce[:8], c.encSeq)
	c.encSeq++
	return c.aead.Seal(nil, nonce, plaintext, nil)
}

func (c *aeadCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	binary.LittleEndian.PutUint64(nonce[:8], c.decSeq)
	out, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	c.decSeq++
	return out, nil
}
