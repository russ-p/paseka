package bus_test

import (
	"testing"

	"github.com/paseka/paseka/internal/bus"
)

func TestPublisherAvailable(t *testing.T) {
	if bus.PublisherAvailable(nil) {
		t.Fatal("nil publisher should be unavailable")
	}
	if !bus.PublisherAvailable(bus.NopPublisher{}) {
		t.Fatal("NopPublisher value should be available")
	}
	var nilClient *bus.Client
	var pub bus.Publisher = nilClient
	if bus.PublisherAvailable(pub) {
		t.Fatal("typed-nil *Client should be unavailable")
	}
}
