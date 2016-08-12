package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type AESStorage struct {
	encryptionKey string
	backingStore  RepositoryReaderWriter
}

func NewAESStorage(key string, backingStore RepositoryReaderWriter) AESStorage {
	return AESStorage{
		encryptionKey: key,
		backingStore:  backingStore,
	}
}

func (f AESStorage) Store(r Repository) error {
	// either 16, 24, or 32 bytes
	block, err := aes.NewCipher([]byte(f.encryptionKey))
	if err != nil {
		return err
	}

	fmt.Printf("%#v\n", f.encryptionKey)
	var buf = &bytes.Buffer{}
	json.NewEncoder(buf).Encode(r)

	encrypted := make([]byte, aes.BlockSize+buf.Len())
	iv := encrypted[:aes.BlockSize]

	encrypter := cipher.NewCFBEncrypter(block, iv)
	encrypter.XORKeyStream(encrypted[aes.BlockSize:], buf.Bytes())

	return f.backingStore.Store(Repository{
		ID:          r.ID,
		AccessToken: base64.StdEncoding.EncodeToString(encrypted),
		Plugins:     r.Plugins,
	})
}

func (f AESStorage) Load() ([]Repository, error) {
	repos, err := f.backingStore.Load()
	if err != nil {
		return nil, err
	}

	fmt.Printf("%#v\n", f.encryptionKey)
	for i, repo := range repos {
		block, err := aes.NewCipher([]byte(f.encryptionKey))
		if err != nil {
			return nil, err
		}

		decoded, _ := base64.StdEncoding.DecodeString(repo.AccessToken)
		encrypted := decoded
		iv := encrypted[:aes.BlockSize]

		encrypted = encrypted[aes.BlockSize:]
		decrypter := cipher.NewCFBDecrypter(block, iv)

		decrypted := make([]byte, len(encrypted))
		decrypter.XORKeyStream(decrypted, encrypted)

		var r Repository
		fmt.Printf("%v\n", string(decrypted))
		json.NewDecoder(bytes.NewBuffer(decrypted)).Decode(&r)
		repos[i] = r
	}

	return repos, nil
}
