package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultClient(t *testing.T) {
	newClient, err := NewClient(DefaultVeneurAddress)
	assert.NoError(t, err)

	defer initializeDefaultClient()

	var nilClient *Client = nil

	// Setting nil works
	SetDefaultClient(nilClient)
	assert.Equal(t, nilClient, DefaultClient)

	// Replacing nil and setting a client works
	SetDefaultClient(newClient)
	assert.Equal(t, newClient, DefaultClient)
}
