package notifications

import (
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Live TV is not part of Polyfin beta: nothing records or reminds, so
// these events are never sent. Their types stay so that saved targets and
// their event lists keep loading, and the delivery tests keep their
// sample events.

// Ended is a recording that ended (formerly recordings.Ended).
type Ended struct {
	Recording  accounts.ID
	User       *accounts.ID
	Channel    accounts.ID
	Name       string
	Start, End time.Time
	Partial    bool
	Failed     bool
	NoTuner    bool
}

// ProgrammeJSON is the Live TV programme a reminder told of.
type ProgrammeJSON struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ChannelID   string    `json:"channelId"`
	ChannelName string    `json:"channelName"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}
