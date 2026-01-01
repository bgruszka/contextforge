package generator

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUUIDGenerator(t *testing.T) {
	gen := &UUIDGenerator{}

	// UUID v4 pattern: 8-4-4-4-12 hex digits
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for i := 0; i < 100; i++ {
		value := gen.Generate()
		assert.Regexp(t, uuidPattern, value, "Generated value should be a valid UUID v4")
	}

	// Test uniqueness
	values := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		value := gen.Generate()
		assert.False(t, values[value], "UUIDs should be unique")
		values[value] = true
	}
}

func TestULIDGenerator(t *testing.T) {
	gen := NewULIDGenerator()

	// ULID pattern: 26 characters from Crockford's Base32
	ulidPattern := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

	for i := 0; i < 100; i++ {
		value := gen.Generate()
		assert.Len(t, value, 26, "ULID should be 26 characters")
		assert.Regexp(t, ulidPattern, value, "Generated value should be a valid ULID")
	}

	// Test uniqueness
	values := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		value := gen.Generate()
		assert.False(t, values[value], "ULIDs should be unique")
		values[value] = true
	}

	// Test lexicographic ordering (ULIDs generated later should be greater)
	ulid1 := gen.Generate()
	time.Sleep(2 * time.Millisecond)
	ulid2 := gen.Generate()
	assert.Greater(t, ulid2, ulid1, "Later ULID should be lexicographically greater")
}

func TestTimestampGenerator(t *testing.T) {
	gen := &TimestampGenerator{}

	before := time.Now().UTC()
	value := gen.Generate()
	after := time.Now().UTC()

	// Parse the generated timestamp
	parsed, err := time.Parse(time.RFC3339Nano, value)
	require.NoError(t, err, "Generated value should be a valid RFC3339Nano timestamp")

	// Check that the timestamp is within the expected range
	assert.True(t, !parsed.Before(before.Truncate(time.Nanosecond)), "Timestamp should not be before the test start")
	assert.True(t, !parsed.After(after.Add(time.Millisecond)), "Timestamp should not be after the test end")
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		genType     Type
		expectError bool
	}{
		{
			name:        "uuid generator",
			genType:     TypeUUID,
			expectError: false,
		},
		{
			name:        "ulid generator",
			genType:     TypeULID,
			expectError: false,
		},
		{
			name:        "timestamp generator",
			genType:     TypeTimestamp,
			expectError: false,
		},
		{
			name:        "unknown generator type",
			genType:     Type("unknown"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := New(tt.genType)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, gen)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, gen)
				// Verify the generator works
				value := gen.Generate()
				assert.NotEmpty(t, value)
			}
		})
	}
}

func TestULIDGeneratorConcurrency(t *testing.T) {
	gen := NewULIDGenerator()
	values := make(chan string, 1000)

	// Generate ULIDs concurrently
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				values <- gen.Generate()
			}
		}()
	}

	// Collect all values
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		value := <-values
		assert.Len(t, value, 26, "ULID should be 26 characters")
		assert.False(t, seen[value], "ULIDs should be unique even under concurrent access")
		seen[value] = true
	}
}
