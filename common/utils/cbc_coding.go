package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padText...)
}

func PKCS5UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	unPadding := int(origData[length-1])
	if unPadding >= length {
		return nil, fmt.Errorf("decrypt error, unpadding more than length")
	}
	for i := length - 1; i > length-unPadding; i-- {
		if int(origData[i]) != unPadding {
			return nil, fmt.Errorf("decrpt error, unpadding format not legal")
		}
	}
	return origData[:(length - unPadding)], nil
}

func CbcEncrypt(src []byte, key []byte, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	_src := PKCS5Padding(src, block.BlockSize())
	dst := make([]byte, len(_src))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(dst, _src)
	return base64.StdEncoding.EncodeToString(dst), nil
}

func CbcDecrypt(src []byte, key []byte, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	dst := make([]byte, len(src))

	if len(iv) != block.BlockSize() {
		return "", fmt.Errorf("cipher.NewCBCDecrypter: IV length must equal block size")
	}
	mode := cipher.NewCBCDecrypter(block, iv)

	if len(src)%mode.BlockSize() != 0 {
		return "", fmt.Errorf("crypto/cipher: input not full blocks")
	}
	mode.CryptBlocks(dst, src)

	_dst, err := PKCS5UnPadding(dst)
	if err != nil {
		return "", err
	}

	plaintext := string(_dst)

	return plaintext, nil
}
