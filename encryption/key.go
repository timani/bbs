package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
)

//go:generate counterfeiter . Key
type Key interface {
	Label() string
	Block() cipher.Block
}

type key struct {
	block cipher.Block
	label string
}

func NewKey(label, phrase string) (Key, error) {
	if label == "" {
		return nil, errors.New("A key label is required")
	}

	hash := sha256.Sum256([]byte(phrase))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	return &key{
		label: label,
		block: block,
	}, nil
}

func (k *key) Label() string {
	return k.label
}

func (k *key) Block() cipher.Block {
	return k.block
}
