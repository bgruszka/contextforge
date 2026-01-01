// Package generator provides header value generators for the ContextForge proxy.
package generator

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Type represents the type of header value generator.
type Type string

const (
	// TypeUUID generates a UUID v4 value.
	TypeUUID Type = "uuid"
	// TypeULID generates a ULID value.
	TypeULID Type = "ulid"
	// TypeTimestamp generates an RFC3339 timestamp.
	TypeTimestamp Type = "timestamp"
)

// Generator generates header values.
type Generator interface {
	Generate() string
}

// UUIDGenerator generates UUID v4 values.
type UUIDGenerator struct{}

// Generate returns a new UUID v4 string.
func (g *UUIDGenerator) Generate() string {
	return uuid.New().String()
}

// ULIDGenerator generates ULID values.
// ULID is a Universally Unique Lexicographically Sortable Identifier.
type ULIDGenerator struct {
	mu   sync.Mutex
	rand *rand.Rand
}

// NewULIDGenerator creates a new ULID generator.
func NewULIDGenerator() *ULIDGenerator {
	return &ULIDGenerator{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Generate returns a new ULID string.
// ULID format: 26 character string (10 chars timestamp + 16 chars randomness)
// Example: 01ARZ3NDEKTSV4RRFFQ69G5FAV
func (g *ULIDGenerator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	t := time.Now().UTC()
	return encodeULID(t, g.rand)
}

// TimestampGenerator generates RFC3339 timestamp values.
type TimestampGenerator struct{}

// Generate returns the current time as an RFC3339 string.
func (g *TimestampGenerator) Generate() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// New creates a generator for the specified type.
// Returns an error if the type is not recognized.
func New(genType Type) (Generator, error) {
	switch genType {
	case TypeUUID:
		return &UUIDGenerator{}, nil
	case TypeULID:
		return NewULIDGenerator(), nil
	case TypeTimestamp:
		return &TimestampGenerator{}, nil
	default:
		return nil, fmt.Errorf("unknown generator type: %s (valid types: uuid, ulid, timestamp)", genType)
	}
}

// Crockford's Base32 encoding alphabet (excludes I, L, O, U to avoid confusion)
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// encodeULID generates a ULID from timestamp and random source.
// ULID specification: https://github.com/ulid/spec
func encodeULID(t time.Time, r *rand.Rand) string {
	// ULID is 26 characters: 10 for timestamp (48-bit ms), 16 for randomness (80-bit)
	ulid := make([]byte, 26)

	// Encode timestamp (48-bit milliseconds since Unix epoch)
	ms := uint64(t.UnixMilli())

	// Timestamp encoding (10 characters, 5 bits each = 50 bits, but only use 48)
	ulid[0] = crockfordAlphabet[(ms>>45)&0x1F]
	ulid[1] = crockfordAlphabet[(ms>>40)&0x1F]
	ulid[2] = crockfordAlphabet[(ms>>35)&0x1F]
	ulid[3] = crockfordAlphabet[(ms>>30)&0x1F]
	ulid[4] = crockfordAlphabet[(ms>>25)&0x1F]
	ulid[5] = crockfordAlphabet[(ms>>20)&0x1F]
	ulid[6] = crockfordAlphabet[(ms>>15)&0x1F]
	ulid[7] = crockfordAlphabet[(ms>>10)&0x1F]
	ulid[8] = crockfordAlphabet[(ms>>5)&0x1F]
	ulid[9] = crockfordAlphabet[ms&0x1F]

	// Randomness encoding (16 characters, 5 bits each = 80 bits)
	// Generate 10 bytes (80 bits) of randomness
	randomBytes := make([]byte, 10)
	r.Read(randomBytes)

	// Encode randomness using Crockford's Base32
	ulid[10] = crockfordAlphabet[(randomBytes[0]>>3)&0x1F]
	ulid[11] = crockfordAlphabet[((randomBytes[0]<<2)|(randomBytes[1]>>6))&0x1F]
	ulid[12] = crockfordAlphabet[(randomBytes[1]>>1)&0x1F]
	ulid[13] = crockfordAlphabet[((randomBytes[1]<<4)|(randomBytes[2]>>4))&0x1F]
	ulid[14] = crockfordAlphabet[((randomBytes[2]<<1)|(randomBytes[3]>>7))&0x1F]
	ulid[15] = crockfordAlphabet[(randomBytes[3]>>2)&0x1F]
	ulid[16] = crockfordAlphabet[((randomBytes[3]<<3)|(randomBytes[4]>>5))&0x1F]
	ulid[17] = crockfordAlphabet[randomBytes[4]&0x1F]
	ulid[18] = crockfordAlphabet[(randomBytes[5]>>3)&0x1F]
	ulid[19] = crockfordAlphabet[((randomBytes[5]<<2)|(randomBytes[6]>>6))&0x1F]
	ulid[20] = crockfordAlphabet[(randomBytes[6]>>1)&0x1F]
	ulid[21] = crockfordAlphabet[((randomBytes[6]<<4)|(randomBytes[7]>>4))&0x1F]
	ulid[22] = crockfordAlphabet[((randomBytes[7]<<1)|(randomBytes[8]>>7))&0x1F]
	ulid[23] = crockfordAlphabet[(randomBytes[8]>>2)&0x1F]
	ulid[24] = crockfordAlphabet[((randomBytes[8]<<3)|(randomBytes[9]>>5))&0x1F]
	ulid[25] = crockfordAlphabet[randomBytes[9]&0x1F]

	return string(ulid)
}
