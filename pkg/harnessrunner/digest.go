package harnessrunner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func digestJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func signJSON(key []byte, v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(b)
	return hex.EncodeToString(m.Sum(nil)), nil
}

func verifyJSON(key []byte, v any, token string) bool {
	want, err := signJSON(key, v)
	return err == nil && hmac.Equal([]byte(want), []byte(token))
}
