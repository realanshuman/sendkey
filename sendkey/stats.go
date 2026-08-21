package sendkey

import (
	"context"
	"time"
)

// Stats is the aggregate counter set behind /stats.
//
// Every field is a running total. There is deliberately nothing here that
// describes one secret or one person: no identifiers, no addresses, no user
// agents, no per-event timestamps, no sizes attached to a particular link.
// Counting how many secrets were created is a different act from recording
// who created them, and only the first one happens.
//
// The set is also bounded by what the server can honestly see. It stores
// opaque ciphertext, so it cannot report how many secrets were files rather
// than text, how large any one of them was, or what any of them contained.
// Those numbers are absent because they are unknowable here, not because
// they were left for later.
type Stats struct {
	Created  int64 `json:"created"`  // links created
	Opened   int64 `json:"opened"`   // reads that handed back ciphertext
	Burned   int64 `json:"burned"`   // links that spent their final view
	Answered int64 `json:"answered"` // ask mailboxes filled in
	Sealed   int64 `json:"sealed"`   // ciphertext bytes accepted, in total

	// How senders set the two dials, as counts of each choice.
	TTLHour int64 `json:"ttlHour"`
	TTLDay  int64 `json:"ttlDay"`
	TTLWeek int64 `json:"ttlWeek"`
	Views1  int64 `json:"views1"`
	Views2  int64 `json:"views2"`
	Views5  int64 `json:"views5"`

	// Since is when this counter set started. In-memory storage restarts with
	// the process, so the page can say what window the numbers cover instead
	// of implying they run back to launch.
	Since time.Time `json:"since"`
}

// ttlBucket maps a clamped TTL onto the three choices the composer offers.
// Anything between the presets rounds to the nearest one below it, so the
// three counts always sum to Created.
func ttlBucket(ttl time.Duration) string {
	switch {
	case ttl <= time.Hour:
		return "ttlHour"
	case ttl <= 24*time.Hour:
		return "ttlDay"
	default:
		return "ttlWeek"
	}
}

// viewsBucket maps a clamped view count onto the composer's three choices,
// on the same round-down rule as ttlBucket.
func viewsBucket(views int) string {
	switch {
	case views <= 1:
		return "views1"
	case views <= 2:
		return "views2"
	default:
		return "views5"
	}
}

// statsReader is the read half of the counters, split out so handlers can
// depend on just this.
type statsReader interface {
	Stats(ctx context.Context) (Stats, error)
}
