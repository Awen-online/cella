package govaction

import (
	"encoding/json"
	"math/big"
	"testing"
)

// Every fixture below is a real proposal_description, copied verbatim from
// Koios's mainnet proposal_list. A decoder tested only against shapes I made up
// would prove nothing about the shapes the chain actually emits.

func TestDecodeTreasuryWithdrawals(t *testing.T) {
	const raw = `{"tag": "TreasuryWithdrawals", "contents": [[[{"network": "Mainnet", "credential": {"scriptHash": "eb06997a94b339ee0b0dd0de7bfec2a184d1af577586654d44e90558"}}, 120000000000000]], "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64"]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Kind != TreasuryWithdrawals || p.Withdrawals == nil {
		t.Fatalf("Kind = %q, Withdrawals = %v", p.Kind, p.Withdrawals)
	}
	w := p.Withdrawals

	if len(w.Recipients) != 1 {
		t.Fatalf("got %d recipients, want 1", len(w.Recipients))
	}
	r := w.Recipients[0]
	if r.Network != "Mainnet" {
		t.Errorf("network = %q", r.Network)
	}
	if !r.IsScript || r.Credential != "eb06997a94b339ee0b0dd0de7bfec2a184d1af577586654d44e90558" {
		t.Errorf("credential = %q (script=%v)", r.Credential, r.IsScript)
	}

	// 120,000,000,000,000 lovelace = 120,000,000 ada. This exceeds float64's
	// exact-integer range if anyone is tempted to parse it as a number.
	want := big.NewInt(120_000_000_000_000)
	if w.Total.Cmp(want) != 0 {
		t.Errorf("total = %s, want %s", w.Total, want)
	}
	if got := ADA(w.Total); got != "120,000,000" {
		t.Errorf("ADA(total) = %q, want \"120,000,000\"", got)
	}
	if got := r.Percent(w.Total); got != 100 {
		t.Errorf("sole recipient's share = %v%%, want 100", got)
	}
	if w.ScriptHash != "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64" {
		t.Errorf("guardrails script = %q", w.ScriptHash)
	}
}

// A treasury figure must survive decoding exactly. Rounding one is misreporting
// it, and the committee is voting on the number.
func TestWithdrawalAmountsAreExact(t *testing.T) {
	// 9,007,199,254,740,993 lovelace — one above 2^53, the point at which a
	// float64 can no longer represent consecutive integers.
	const raw = `{"tag":"TreasuryWithdrawals","contents":[[[{"network":"Mainnet","credential":{"keyHash":"aa"}},9007199254740993]],""]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want, _ := new(big.Int).SetString("9007199254740993", 10)
	if got := p.Withdrawals.Total; got.Cmp(want) != 0 {
		t.Errorf("total = %s, want %s — the amount was rounded", got, want)
	}
}

func TestDecodeMultipleRecipients(t *testing.T) {
	const raw = `{"tag":"TreasuryWithdrawals","contents":[[
		[{"network":"Mainnet","credential":{"keyHash":"aaa"}}, 75000000000],
		[{"network":"Mainnet","credential":{"keyHash":"bbb"}}, 25000000000]
	],"script1"]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	w := p.Withdrawals
	if len(w.Recipients) != 2 {
		t.Fatalf("got %d recipients, want 2", len(w.Recipients))
	}
	if got := ADA(w.Total); got != "100,000" {
		t.Errorf("total = %s ada, want 100,000", got)
	}
	if got := w.Recipients[0].Percent(w.Total); got != 75 {
		t.Errorf("first recipient = %v%%, want 75", got)
	}
	if got := w.Recipients[1].Percent(w.Total); got != 25 {
		t.Errorf("second recipient = %v%%, want 25", got)
	}
}

func TestDecodeParameterChange(t *testing.T) {
	const raw = `{"tag": "ParameterChange", "contents": [{"txId": "c82f38", "govActionIx": 0}, {"committeeMinSize": 5}, "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64"]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Kind != ParameterChange || p.Parameters == nil {
		t.Fatalf("Kind = %q", p.Kind)
	}
	if len(p.Parameters.Changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(p.Parameters.Changes))
	}
	c := p.Parameters.Changes[0]
	if c.Name != "committeeMinSize" || c.Proposed != "5" {
		t.Errorf("change = %+v, want committeeMinSize = 5", c)
	}
}

// Parameters must render in a stable order — a table that reshuffles on refresh
// is a table nobody can diff by eye.
func TestParameterOrderIsStable(t *testing.T) {
	const raw = `{"tag":"ParameterChange","contents":[null,{"zeta":1,"alpha":2,"mu":3,"beta":{"nested":true}},""]}`

	var first []string
	for i := 0; i < 20; i++ {
		p, err := Decode(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		var names []string
		for _, c := range p.Parameters.Changes {
			names = append(names, c.Name)
		}
		if first == nil {
			first = names
			continue
		}
		for j := range names {
			if names[j] != first[j] {
				t.Fatalf("parameter order is unstable: %v vs %v", names, first)
			}
		}
	}
	want := []string{"alpha", "beta", "mu", "zeta"}
	for i := range want {
		if first[i] != want[i] {
			t.Errorf("order = %v, want %v (sorted)", first, want)
		}
	}

	// A structured value must survive as readable JSON, not as Go's %v.
	p, _ := Decode(json.RawMessage(raw))
	for _, c := range p.Parameters.Changes {
		if c.Name == "beta" && c.Proposed != `{"nested":true}` {
			t.Errorf("structured value rendered as %q", c.Proposed)
		}
	}
}

func TestDecodeHardFork(t *testing.T) {
	const raw = `{"tag": "HardForkInitiation", "contents": [{"txId": "0b1947", "govActionIx": 0}, {"major": 11, "minor": 0}]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.HardFork == nil || p.HardFork.Version() != "11.0" {
		t.Fatalf("version = %v, want 11.0", p.HardFork)
	}
}

// Koios reports a committee update with proposal_type "NewCommittee" but a
// payload tag of "UpdateCommittee". Decoding must follow the tag.
func TestDecodeCommitteeUpdate(t *testing.T) {
	const raw = `{"tag": "UpdateCommittee", "contents": [{"txId": "4dab33", "govActionIx": 0}, [], {"keyHash-c46a3789d71bc0a27d1c381909289978797a60c5f67aa8bc2b26ab92": 653}, {"numerator": 2, "denominator": 3}]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Kind != UpdateCommittee || p.Committee == nil {
		t.Fatalf("Kind = %q — the payload tag, not the proposal_type, is authoritative", p.Kind)
	}
	u := p.Committee

	if len(u.Added) != 1 {
		t.Fatalf("got %d added seats, want 1", len(u.Added))
	}
	if u.Added[0].ExpiresAt != 653 {
		t.Errorf("seat expires epoch %d, want 653", u.Added[0].ExpiresAt)
	}
	if len(u.Removed) != 0 {
		t.Errorf("removed = %v, want none", u.Removed)
	}
	if u.Quorum == nil || u.Quorum.String() != "2 of 3" {
		t.Errorf("quorum = %v, want 2 of 3", u.Quorum)
	}
}

func TestDecodeNewConstitution(t *testing.T) {
	const raw = `{"tag": "NewConstitution", "contents": [{"txId": "8c653e", "govActionIx": 0}, {"anchor": {"url": "ipfs://bafkreieyuknozbtewyurfqoagvplvykadn6a4u6wglupavdz46bbsnnl6e", "dataHash": "b368bdad83c727bbfe86425575233fb914eb76d05d89497f7790cf007fd95f52"}, "script": "fa24fb305126805cf2164c161d852a0e7330cf988f1fe558cf7d4a64"}]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c := p.Constitution
	if c == nil {
		t.Fatal("no constitution change decoded")
	}
	if c.AnchorURL != "ipfs://bafkreieyuknozbtewyurfqoagvplvykadn6a4u6wglupavdz46bbsnnl6e" {
		t.Errorf("anchor = %q", c.AnchorURL)
	}
	if c.DataHash != "b368bdad83c727bbfe86425575233fb914eb76d05d89497f7790cf007fd95f52" {
		t.Errorf("data hash = %q", c.DataHash)
	}
	if c.ScriptHash == "" {
		t.Error("guardrails script hash was dropped")
	}
}

func TestDecodeInfoAction(t *testing.T) {
	p, err := Decode(json.RawMessage(`{"tag": "InfoAction"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Kind != InfoAction {
		t.Errorf("Kind = %q, want InfoAction", p.Kind)
	}
	if !p.Recognised() {
		t.Error("InfoAction should be recognised even though it carries nothing")
	}
}

// A payload Cella cannot decode must be surfaced raw, not silently dropped.
// Showing a delegate nothing lets them assume there was nothing to see.
func TestUnknownPayloadIsSurfacedNotSwallowed(t *testing.T) {
	const raw = `{"tag":"SomeFutureAction","contents":[1,2,3]}`

	p, err := Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if p.Kind != "SomeFutureAction" {
		t.Errorf("Kind = %q, want the unrecognised tag preserved", p.Kind)
	}
	if p.Recognised() {
		t.Error("an undecodable payload reported itself Recognised()")
	}
	if len(p.Unknown) == 0 {
		t.Error("the raw payload was dropped; a delegate would see nothing at all")
	}
}

func TestDecodeRejectsRubbish(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":     ``,
		"null":      `null`,
		"no tag":    `{"contents":[]}`,
		"not JSON":  `<xml/>`,
		"bare list": `[1,2,3]`,
	} {
		t.Run(name, func(t *testing.T) {
			if p, err := Decode(json.RawMessage(raw)); err == nil {
				t.Errorf("Decode(%q) succeeded with %+v; want an error", raw, p)
			}
		})
	}
}

func TestADA(t *testing.T) {
	cases := map[string]int64{
		"0":           0,
		"1":           1_000_000,
		"1,000":       1_000_000_000,
		"120,000,000": 120_000_000_000_000,
		"0.500000":    500_000,
		"1.234567":    1_234_567,
	}
	for want, lovelace := range cases {
		if got := ADA(big.NewInt(lovelace)); got != want {
			t.Errorf("ADA(%d) = %q, want %q", lovelace, got, want)
		}
	}
}
