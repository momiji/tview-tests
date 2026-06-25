// Package secret encrypts and decrypts configuration passwords using a
// locally stored key file: an AES-256 key is derived (via MD5) from the
// bytes of the key file, and payloads are AES-GCM sealed and
// base64-encoded.
//
// The key file location is injected through New rather than read from a
// global, so the domain stays free of CLI/global state. The interactive
// command that produces encrypted passwords from a prompt is not part of
// this package; it belongs to the CLI layer and calls Encrypt with Prefix.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
)

// Prefix marks a value as encrypted in configuration files: an encrypted
// password is stored as Prefix + base64(ciphertext). Callers strip Prefix
// before Decrypt and prepend it after Encrypt.
const Prefix = "encrypted:"

// Cipher encrypts and decrypts strings with the key stored in keyFile.
type Cipher struct {
	keyFile string
}

// New returns a Cipher that reads (and, if missing, creates) its key from
// keyFile.
func New(keyFile string) *Cipher {
	return &Cipher{keyFile: keyFile}
}

func (c *Cipher) createKey() []byte {
	key := make([]byte, 256)
	rand.Read(key)
	return key
}

func (c *Cipher) readKey() []byte {
	key, err := os.ReadFile(c.keyFile)
	if err == nil {
		return key
	}
	key = c.createKey()
	err = os.WriteFile(c.keyFile, key, 0600)
	if err != nil {
		panic(err)
	}
	return key
}

func (c *Cipher) createHash() string {
	hasher := md5.New()
	hasher.Write(c.readKey())
	return hex.EncodeToString(hasher.Sum(nil))
}

// Encrypt seals data with AES-GCM and returns the base64-encoded
// ciphertext (without Prefix).
func (c *Cipher) Encrypt(data string) string {
	block, _ := aes.NewCipher([]byte(c.createHash()))
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
	encrypted := base64.StdEncoding.EncodeToString(ciphertext)
	return encrypted
}

// Decrypt reverses Encrypt: it base64-decodes data (which must not include
// Prefix) and opens the AES-GCM ciphertext.
func (c *Cipher) Decrypt(data string) (string, error) {
	encoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	key := []byte(c.createHash())
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := encoded[:nonceSize], encoded[nonceSize:]
	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}
