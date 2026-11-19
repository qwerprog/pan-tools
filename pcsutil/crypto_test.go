package pcsutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptAndDecryptFileAllModes(t *testing.T) {
	methods := []string{
		"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
		"aes-128-ofb", "aes-192-ofb", "aes-256-ofb",
	}
	payload := []byte("pan-tools pure stdlib AES + GZIP file encryption round-trip payload\n")
	keys := [][]byte{
		[]byte("pan-tools"),
		[]byte("a-very-long-key-exceeding-32-bytes-for-truncation-verification-123456"),
	}

	for _, method := range methods {
		if !CryptoMethodSupport(method) {
			t.Fatalf("expected method %q to be supported", method)
		}
		for _, isGzip := range []bool{true, false} {
			for _, key := range keys {
				dir := t.TempDir()
				src := filepath.Join(dir, "sample.txt")
				if err := os.WriteFile(src, payload, 0600); err != nil {
					t.Fatal(err)
				}

				encPath, err := EncryptFile(method, key, src, isGzip)
				if err != nil {
					t.Fatalf("EncryptFile(%s, gzip=%v) failed: %v", method, isGzip, err)
				}
				if encPath != src+".encrypt" {
					t.Fatalf("unexpected encrypted path %q", encPath)
				}
				if _, err := os.Stat(src); !os.IsNotExist(err) {
					t.Fatalf("expected original file to be removed after EncryptFile")
				}

				decPath, err := DecryptFile(method, key, encPath, isGzip)
				if err != nil {
					t.Fatalf("DecryptFile(%s, gzip=%v) failed: %v", method, isGzip, err)
				}
				if decPath != src {
					t.Fatalf("unexpected decrypted path %q, want %q", decPath, src)
				}
				got, err := os.ReadFile(decPath)
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatalf("round-trip mismatch for %s (gzip=%v): err=%v got=%q", method, isGzip, err, string(got))
				}
			}
		}
	}
}

func TestDecryptFileWrongKeyPreservesCiphertext(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(src, []byte("confidential data"), 0600); err != nil {
		t.Fatal(err)
	}

	encPath, err := EncryptFile("aes-128-ctr", []byte("right-key"), src, true)
	if err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}

	if _, err := DecryptFile("aes-128-ctr", []byte("wrong-key"), encPath, true); err == nil {
		t.Fatal("expected error when decrypting gzipped ciphertext with wrong key")
	}
	if _, err := os.Stat(encPath); err != nil {
		t.Fatalf("expected ciphertext %q to be preserved after failed decrypt: %v", encPath, err)
	}
	if CryptoMethodSupport("aes-128-ecb") {
		t.Fatal("unexpected support for aes-128-ecb")
	}
}
