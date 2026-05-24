package apikey

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	plain := "test-key-abc-123"

	hashed, err := Hash(plain)
	require.NoError(t, err)
	assert.NotEqual(t, plain, hashed, "hash must not equal plaintext")
	assert.NotEmpty(t, hashed)

	assert.True(t, Verify(plain, hashed), "correct plaintext must verify")
	assert.False(t, Verify("wrong-key", hashed), "wrong plaintext must not verify")
}

func TestHashIsSalted(t *testing.T) {
	plain := "same-plaintext"
	h1, _ := Hash(plain)
	h2, _ := Hash(plain)
	assert.NotEqual(t, h1, h2, "bcrypt must salt — two hashes of same input differ")
	assert.True(t, Verify(plain, h1))
	assert.True(t, Verify(plain, h2))
}
