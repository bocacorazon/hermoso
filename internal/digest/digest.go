package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
)

func Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func JSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return Bytes(data), nil
}

func Reader(reader io.Reader) (string, error) {
	sum := sha256.New()
	if _, err := io.Copy(sum, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}
