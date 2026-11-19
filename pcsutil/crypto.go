package pcsutil

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"os"
	"strings"
)

// CryptoMethodSupport 检测是否支持加密解密方法
func CryptoMethodSupport(method string) bool {
	_, _, err := parseAESMethod(method)
	return err == nil
}

func parseAESMethod(method string) (keyLen int, mode string, err error) {
	switch method {
	case "aes-128-ctr":
		return 16, "ctr", nil
	case "aes-192-ctr":
		return 24, "ctr", nil
	case "aes-256-ctr":
		return 32, "ctr", nil
	case "aes-128-cfb":
		return 16, "cfb", nil
	case "aes-192-cfb":
		return 24, "cfb", nil
	case "aes-256-cfb":
		return 32, "cfb", nil
	case "aes-128-ofb":
		return 16, "ofb", nil
	case "aes-192-ofb":
		return 24, "ofb", nil
	case "aes-256-ofb":
		return 32, "ofb", nil
	default:
		return 0, "", fmt.Errorf("unknown crypto method: %s", method)
	}
}

// normalizeAESKey 将任意长度密钥规整为目标长度（超长截断，不足补 '\x00'）。
func normalizeAESKey(key []byte, size int) []byte {
	out := make([]byte, size)
	copy(out, key)
	return out
}

func newAESStreamEncryptReader(mode string, key []byte, plainReader io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	var iv [aes.BlockSize]byte
	if _, err = io.ReadFull(cryptorand.Reader, iv[:]); err != nil {
		return nil, fmt.Errorf("generate iv failed: %w", err)
	}

	var stream cipher.Stream
	switch mode {
	case "ctr":
		stream = cipher.NewCTR(block, iv[:])
	case "cfb":
		stream = cipher.NewCFBEncrypter(block, iv[:])
	case "ofb":
		stream = cipher.NewOFB(block, iv[:])
	default:
		return nil, fmt.Errorf("unsupported aes mode: %s", mode)
	}

	return io.MultiReader(bytes.NewReader(iv[:]), &cipher.StreamReader{
		S: stream,
		R: plainReader,
	}), nil
}

func newAESStreamDecryptReader(mode string, key []byte, cipherReader io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	var iv [aes.BlockSize]byte
	if _, err = io.ReadFull(cipherReader, iv[:]); err != nil {
		return nil, err
	}

	var stream cipher.Stream
	switch mode {
	case "ctr":
		stream = cipher.NewCTR(block, iv[:])
	case "cfb":
		stream = cipher.NewCFBDecrypter(block, iv[:])
	case "ofb":
		stream = cipher.NewOFB(block, iv[:])
	default:
		return nil, fmt.Errorf("unsupported aes mode: %s", mode)
	}

	return &cipher.StreamReader{
		S: stream,
		R: cipherReader,
	}, nil
}

// gzipTransformFile 对本地文件执行原地 GZIP 压缩或解压缩，并在重命名前显式关闭句柄（兼容 Windows）。
func gzipTransformFile(filePath string, compress bool) error {
	src, err := os.Open(filePath)
	if err != nil {
		return err
	}
	srcClosed := false
	defer func() {
		if !srcClosed {
			_ = src.Close()
		}
	}()

	fInfo, err := src.Stat()
	if err != nil {
		return err
	}

	tmpPath := filePath + ".gzip.tmp"
	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fInfo.Mode())
	if err != nil {
		return err
	}
	dstClosed := false
	cleanupTmp := func() {
		if !dstClosed {
			dstClosed = true
			_ = dst.Close()
		}
		_ = os.Remove(tmpPath)
	}

	if compress {
		gw := gzip.NewWriter(dst)
		if _, err = io.Copy(gw, src); err != nil {
			_ = gw.Close()
			cleanupTmp()
			return err
		}
		if err = gw.Close(); err != nil {
			cleanupTmp()
			return err
		}
	} else {
		gr, grErr := gzip.NewReader(src)
		if grErr != nil {
			cleanupTmp()
			return grErr
		}
		if _, err = io.Copy(dst, gr); err != nil {
			_ = gr.Close()
			cleanupTmp()
			return err
		}
		if err = gr.Close(); err != nil {
			cleanupTmp()
			return err
		}
	}

	dstClosed = true
	if err = dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	srcClosed = true
	if err = src.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err = os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// EncryptFile 加密本地文件
func EncryptFile(method string, key []byte, filePath string, isGzip bool) (encryptedFilePath string, err error) {
	keyLen, mode, err := parseAESMethod(method)
	if err != nil {
		return "", fmt.Errorf("unknown encrypt method: %s", method)
	}

	if isGzip {
		if err = gzipTransformFile(filePath, true); err != nil {
			return "", err
		}
	}

	plainFile, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	plainClosed := false
	defer func() {
		if !plainClosed {
			_ = plainFile.Close()
		}
	}()

	cipherReader, err := newAESStreamEncryptReader(mode, normalizeAESKey(key, keyLen), plainFile)
	if err != nil {
		return "", err
	}

	plainFileInfo, err := plainFile.Stat()
	if err != nil {
		return "", err
	}

	encryptedFilePath = filePath + ".encrypt"
	encryptedFile, err := os.OpenFile(encryptedFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, plainFileInfo.Mode())
	if err != nil {
		return "", err
	}
	encClosed := false
	defer func() {
		if !encClosed {
			_ = encryptedFile.Close()
		}
	}()

	if _, err = io.Copy(encryptedFile, cipherReader); err != nil {
		return "", err
	}

	encClosed = true
	if err = encryptedFile.Close(); err != nil {
		return "", err
	}
	plainClosed = true
	_ = plainFile.Close()

	_ = os.Remove(filePath)
	return encryptedFilePath, nil
}

// DecryptFile 解密本地文件
func DecryptFile(method string, key []byte, filePath string, isGzip bool) (decryptedFilePath string, err error) {
	keyLen, mode, err := parseAESMethod(method)
	if err != nil {
		return "", fmt.Errorf("unknown decrypt method: %s", method)
	}

	cipherFile, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		return "", err
	}
	cipherClosed := false
	defer func() {
		if !cipherClosed {
			_ = cipherFile.Close()
		}
	}()

	plainReader, err := newAESStreamDecryptReader(mode, normalizeAESKey(key, keyLen), cipherFile)
	if err != nil {
		return "", err
	}

	cipherFileInfo, err := cipherFile.Stat()
	if err != nil {
		return "", err
	}

	decryptedFilePath = strings.TrimSuffix(filePath, ".encrypt")
	decryptedTmpFilePath := decryptedFilePath + ".decrypted"
	decryptedTmpFile, err := os.OpenFile(decryptedTmpFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, cipherFileInfo.Mode())
	if err != nil {
		return "", err
	}
	tmpClosed := false
	defer func() {
		if !tmpClosed {
			_ = decryptedTmpFile.Close()
		}
	}()

	if _, err = io.Copy(decryptedTmpFile, plainReader); err != nil {
		return "", err
	}

	tmpClosed = true
	if err = decryptedTmpFile.Close(); err != nil {
		return "", err
	}
	cipherClosed = true
	_ = cipherFile.Close()

	if isGzip {
		if err = gzipTransformFile(decryptedTmpFilePath, false); err != nil {
			_ = os.Remove(decryptedTmpFilePath)
			return "", err
		}

		// 删除已加密的文件
		_ = os.Remove(filePath)
	}

	if filePath != decryptedFilePath {
		if err = os.Rename(decryptedTmpFilePath, decryptedFilePath); err != nil {
			return "", err
		}
	} else {
		decryptedFilePath = decryptedTmpFilePath
	}

	return decryptedFilePath, nil
}
