package server

import (
	"cmp"
	"fmt"
	"math"
	"time"

	"github.com/Awen-online/cella/internal/koios"
)

// A governance action does not wait for a committee. It expires at a fixed
// epoch, and a committee that misses the deadline has, in effect, abstained
// without meaning to — so the clock is the most operationally urgent fact on
// the page, not a footnote.
//
// The chain states expiration as an epoch *number*, not a time. Turning it into
// a deadline needs the network's genesis parameters, which Cella captures at
// ingest (see store.Network). Without them Cella shows the raw epoch and no
// countdown, rather than inventing a date it cannot justify.

// Deadline is when a governance action stops accepting votes, and how long that
// leaves.
type Deadline struct {
	Epoch int64     // the expiration epoch, as the chain states it
	At    time.Time // the instant voting closes (the end of that epoch)
	Known bool      // false when the genesis parameters are unavailable

	Left    time.Duration // time remaining; negative once past
	Expired bool
}

// deadlineFor computes the deadline for an action expiring at the given epoch.
//
// Voting closes at the *start* of the expiration epoch, not the end of it. The
// chain is unambiguous about this: for every expired action Koios reports,
// expired_epoch equals expiration exactly — an action stating expiration 640 was
// already marked expired during epoch 640. So the last moment it could be voted
// on was the close of epoch 639, which is the instant epoch 640 begins.
//
// This is worth being pedantic about. Reading it as the *end* of the expiration
// epoch — the intuitive reading, and the one Cella shipped with — hands a
// committee an extra five days of runway that does not exist, on the one number
// they cannot afford to be wrong about.
func deadlineFor(epoch int64, p koios.GenesisParams, now time.Time) Deadline {
	d := Deadline{Epoch: epoch}
	if !p.Valid() {
		return d
	}
	d.Known = true
	d.At = p.EpochStart(epoch)
	d.Left = d.At.Sub(now)
	d.Expired = d.Left <= 0
	return d
}

// Urgency buckets the deadline for display. A committee needs to see at a
// glance which actions are about to run out, not read dates.
//
//	expired  — voting has closed
//	critical — under 2 days
//	soon     — under 5 days (roughly one mainnet epoch)
//	ok       — beyond that
//	unknown  — no genesis parameters, so no honest countdown
func (d Deadline) Urgency() string {
	switch {
	case !d.Known:
		return "unknown"
	case d.Expired:
		return "expired"
	case d.Left < 48*time.Hour:
		return "critical"
	case d.Left < 5*24*time.Hour:
		return "soon"
	default:
		return "ok"
	}
}

// Countdown renders the time remaining in the coarsest unit that is still
// honest — a committee cares whether it has days or hours, and reading
// "4d 6h 12m 9s" takes longer than reading "4 days".
func (d Deadline) Countdown() string {
	if !d.Known {
		return ""
	}
	if d.Expired {
		if past := -d.Left; past < 24*time.Hour {
			return "expired today"
		}
		return fmt.Sprintf("expired %s ago", plural(int(math.Floor((-d.Left).Hours()/24)), "day"))
	}

	switch {
	case d.Left < time.Hour:
		return fmt.Sprintf("%s left", plural(int(math.Ceil(d.Left.Minutes())), "minute"))
	case d.Left < 48*time.Hour:
		return fmt.Sprintf("%s left", plural(int(math.Floor(d.Left.Hours())), "hour"))
	default:
		return fmt.Sprintf("%s left", plural(int(math.Floor(d.Left.Hours()/24)), "day"))
	}
}

// When is the deadline as a date, or the raw epoch when it cannot be resolved.
func (d Deadline) When() string {
	if !d.Known {
		return fmt.Sprintf("expires epoch %d", d.Epoch)
	}
	return fmt.Sprintf("expires epoch %d · %s", d.Epoch, d.At.Format("2 Jan 2006 15:04 MST"))
}

// Unix is the deadline as a unix timestamp, for the browser to tick against.
func (d Deadline) Unix() int64 {
	if !d.Known {
		return 0
	}
	return d.At.Unix()
}

// byUrgency orders actions the way a committee needs to see them, which is not
// the order they arrived in.
//
// What matters is what is about to run out. Live actions come first, soonest
// deadline at the top; actions whose deadline we cannot resolve follow; and
// expired ones sink to the bottom, since nothing can be done about them. Sorting
// by ingest date — which is what the chain hands us — buries an action expiring
// in two days under one that merely arrived yesterday.
func byUrgency(a, b actionView) int {
	rank := func(v actionView) int {
		switch {
		case v.Settled():
			return 3 // the chain has decided: nothing left for the committee to do
		case v.Deadline.Known && !v.Deadline.Expired:
			return 0 // live, with a clock: act on these
		case !v.Deadline.Known:
			return 1 // no clock we can resolve, but still open
		default:
			return 2 // out of time, but the chain has not recorded a fate yet
		}
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}

	switch {
	case a.Deadline.Known && !a.Deadline.Expired:
		// Both live — soonest first.
		return cmp.Compare(a.Deadline.Left, b.Deadline.Left)
	case a.Deadline.Expired && b.Deadline.Expired:
		// Both expired — most recently expired first; older history sinks.
		return cmp.Compare(b.Deadline.Left, a.Deadline.Left)
	default:
		// Neither has a usable clock — fall back to newest first.
		return cmp.Compare(b.BlockTime, a.BlockTime)
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
