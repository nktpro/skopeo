package tlsclientconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTransportDefaults(t *testing.T) {
	t.Run("Verify MaxIdleConnsPerHost", func(t *testing.T) {
		tr := NewTransport()
		// It should be set to 100 to match MaxIdleConns and allow efficient parallelism.
		assert.Equal(t, 100, tr.MaxIdleConnsPerHost, "MaxIdleConnsPerHost should be 100")
	})
}
