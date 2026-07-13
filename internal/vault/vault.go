package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

type Cipher struct{ aead cipher.AEAD }

func New(key string) (*Cipher, error) {
	if len(key) < 16 {
		return nil, errors.New("GOLIVE_ENCRYPTION_KEY must be at least 16 characters")
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}
func (c *Cipher) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, c.aead.Seal(nil, nonce, plain, nil)...), nil
}
func (c *Cipher) Open(data []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid encrypted value")
	}
	return c.aead.Open(nil, data[:n], data[n:], nil)
}
