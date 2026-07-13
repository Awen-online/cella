package server

import (
	"testing"
	"time"

	"github.com/Awen-online/cella/internal/koios"
)

// Cardano mainnet genesis, as Koios reports it.
var mainnet = koios.GenesisParams{SystemStart: 1506203091, EpochLength: 432000}

// The epoch arithmetic is the whole feature: if it is wrong, Cella tells a
// committee it has time it does not have.
//
// Pin it to a value derived independently of this code. Koios reported the
// chain in epoch 642 starting at absolute slot 191980800; Shelley began at slot
// 4492800 at unix 1596059091 with one-second slots, so that epoch began at
// 1596059091 + (191980800 - 4492800) = 1783547091. Genesis multiplication must
// land on the same instant — it does, and that agreement across two unrelated
// derivations is what makes the countdown trustworthy.
func TestEpochStartMatchesChain(t *testing.T) {
	const fromKoiosSlots = int64(1783547091) // 2026-07-08T21:44:51Z

	got := mainnet.EpochStart(642)
	if got.Unix() != fromKoiosSlots {
		t.Errorf("EpochStart(642) = %d (%s), want %d (%s) — derived from Koios slot arithmetic",
			got.Unix(), got.Format(time.RFC3339),
			fromKoiosSlots, time.Unix(fromKoiosSlots, 0).UTC().Format(time.RFC3339))
	}

	// Epochs are contiguous: one ends exactly where the next begins.
	if !mainnet.EpochEnd(642).Equal(mainnet.EpochStart(643)) {
		t.Error("EpochEnd(642) != EpochStart(643); epochs are not contiguous")
	}
	// And a mainnet epoch is five days.
	if d := mainnet.EpochEnd(642).Sub(mainnet.EpochStart(642)); d != 5*24*time.Hour {
		t.Errorf("epoch length = %s, want 120h", d)
	}
}

// An action remains votable *through* its expiration epoch, so the deadline is
// the end of that epoch. Getting this off by one epoch would cost a committee
// five days of runway in either direction.
func TestDeadlineIsEndOfExpirationEpoch(t *testing.T) {
	d := deadlineFor(648, mainnet, mainnet.EpochStart(642))
	if !d.Known {
		t.Fatal("deadline should be known with valid genesis params")
	}
	if want := mainnet.EpochStart(649); !d.At.Equal(want) {
		t.Errorf("deadline for epoch 648 = %s, want %s (the end of epoch 648)",
			d.At.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// Without genesis parameters Cella must say it does not know, not invent a
// date. The bug this replaces rendered every expiration epoch as a unix
// timestamp, so every action claimed to have expired in 1970.
func TestDeadlineUnknownWithoutGenesis(t *testing.T) {
	d := deadlineFor(648, koios.GenesisParams{}, time.Now())

	if d.Known {
		t.Error("deadline claims to be known with no genesis parameters")
	}
	if d.Urgency() != "unknown" {
		t.Errorf("Urgency() = %q, want \"unknown\"", d.Urgency())
	}
	if d.Countdown() != "" {
		t.Errorf("Countdown() = %q, want empty — there is nothing honest to count", d.Countdown())
	}
	if d.Unix() != 0 {
		t.Errorf("Unix() = %d, want 0 so the browser does not tick a fictional clock", d.Unix())
	}
	if want := "end of epoch 648"; d.When() != want {
		t.Errorf("When() = %q, want %q", d.When(), want)
	}
	// It must never claim the action has expired just because it cannot tell.
	if d.Expired {
		t.Error("an unknown deadline was reported as expired")
	}
}

func TestUrgencyAndCountdown(t *testing.T) {
	// Deadline is the end of epoch 648.
	end := mainnet.EpochEnd(648)

	cases := []struct {
		name        string
		now         time.Time
		wantUrgency string
		wantText    string
	}{
		{"plenty of time", end.Add(-20 * 24 * time.Hour), "ok", "20 days left"},
		{"just over five days", end.Add(-6 * 24 * time.Hour), "ok", "6 days left"},
		{"under five days", end.Add(-4 * 24 * time.Hour), "soon", "4 days left"},
		{"under two days", end.Add(-30 * time.Hour), "critical", "30 hours left"},
		{"a single hour", end.Add(-90 * time.Minute), "critical", "1 hour left"},
		{"minutes", end.Add(-40 * time.Minute), "critical", "40 minutes left"},
		{"exactly one day", end.Add(-24 * time.Hour), "critical", "24 hours left"},
		{"just expired", end.Add(time.Minute), "expired", "expired today"},
		{"long expired", end.Add(9 * 24 * time.Hour), "expired", "expired 9 days ago"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := deadlineFor(648, mainnet, tc.now)
			if got := d.Urgency(); got != tc.wantUrgency {
				t.Errorf("Urgency() = %q, want %q", got, tc.wantUrgency)
			}
			if got := d.Countdown(); got != tc.wantText {
				t.Errorf("Countdown() = %q, want %q", got, tc.wantText)
			}
		})
	}
}

// Singulars must read as singulars — "1 days left" on a governance deadline
// looks like a bug and undermines trust in the rest of the number.
//
// Note there is deliberately no "1 day left" case: inside 48 hours the
// countdown switches to hours, which is the more actionable unit when a
// deadline is close, so a remaining day always reads as "26 hours left" rather
// than rounding away the urgency.
func TestCountdownPluralisation(t *testing.T) {
	end := mainnet.EpochEnd(648)
	cases := map[string]time.Duration{
		"2 days left":       50 * time.Hour,
		"26 hours left":     26 * time.Hour,
		"1 hour left":       90 * time.Minute,
		"1 minute left":     30 * time.Second,
		"expired 1 day ago": -30 * time.Hour,
	}
	for want, before := range cases {
		d := deadlineFor(648, mainnet, end.Add(-before))
		if got := d.Countdown(); got != want {
			t.Errorf("at %s from the deadline: Countdown() = %q, want %q", before, got, want)
		}
	}
}
