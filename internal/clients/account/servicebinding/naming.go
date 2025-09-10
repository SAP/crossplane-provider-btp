package servicebindingclient

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var (
	src      = rand.NewSource(time.Now().UnixNano())
	srcMutex sync.Mutex
)

func RandomString(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)

	srcMutex.Lock()
	defer srcMutex.Unlock()

	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

func GenerateRandomName(name string) string {
	if len(name) > 0 && name[len(name)-1] == '-' {
		name = name[:len(name)-1]
	}
	newName := name + "-" + RandomString(5)
	return newName
}

// GenerateInstanceUID creates a deterministic UID by combining the original ServiceBinding UID with the instance name
func GenerateInstanceUID(originalUID types.UID, instanceName string) types.UID {
	combined := string(originalUID) + "-" + instanceName
	return types.UID(combined)
}