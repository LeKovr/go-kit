package observability

import (
	"sort"
	"strings"

	"github.com/nats-io/nats.go"
)

// NATS preserves header casing on the wire, unlike net/http.Header.
type natsHeaderCarrier nats.Header

func (h natsHeaderCarrier) Get(key string) string {
	if values := h.Values(key); len(values) > 0 {
		return values[0]
	}

	return ""
}

func (h natsHeaderCarrier) Set(key, value string) {
	h[strings.ToLower(key)] = []string{value}
}

func (h natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		keys = append(keys, key)
	}

	return keys
}

// Values implements propagation.ValuesGetter, preserving all baggage fields.
// Sorting makes the order of case variants deterministic.
func (h natsHeaderCarrier) Values(key string) []string {
	var keys []string
	for candidate := range h {
		if strings.EqualFold(key, candidate) {
			keys = append(keys, candidate)
		}
	}

	sort.Strings(keys)
	var values []string
	for _, candidate := range keys {
		values = append(values, h[candidate]...)
	}

	return values
}
