package common

import (
	"crypto/sha256"
	"encoding/hex"
)

// FuncID  获取witstone语言函数ID
func FuncID(name string, inputType ...string) string {
	h := sha256.New()
	h.Write([]byte(name))
	for _, p := range inputType {
		h.Write([]byte(p))
	}
	dst := make([]byte, hex.EncodedLen(h.Size()))
	hex.Encode(dst, h.Sum(nil))
	return string(dst[:8])
}
