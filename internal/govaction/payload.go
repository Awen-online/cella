// Package govaction decodes what a governance action actually *does* — the
// on-chain payload the chain will execute if the action is enacted.
//
// Everything else Cella shows about an action is what its authors said about
// it: a title, an abstract, a motivation, all of it off-chain prose that anyone
// can write. The payload is the part that is binding. A committee assessing
// constitutionality has to read the payload, not the pitch, so Cella decodes it
// rather than leaving the delegate to trust the summary.
package govaction

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
)

// Kind is the payload's discriminator, as the ledger names it.
//
// Note this is NOT always the same string as Koios's proposal_type: a committee
// update has proposal_type "NewCommittee" but a payload tag of
// "UpdateCommittee". Trust the tag.
type Kind string

const (
	TreasuryWithdrawals Kind = "TreasuryWithdrawals"
	ParameterChange     Kind = "ParameterChange"
	HardForkInitiation  Kind = "HardForkInitiation"
	UpdateCommittee     Kind = "UpdateCommittee"
	NewConstitution     Kind = "NewConstitution"
	NoConfidence        Kind = "NoConfidence"
	InfoAction          Kind = "InfoAction"
)

// Payload is a decoded governance action payload. Exactly one of the typed
// fields is populated, according to Kind.
type Payload struct {
	Kind Kind

	Withdrawals  *Withdrawals
	Parameters   *Parameters
	HardFork     *HardFork
	Committee    *CommitteeUpdate
	Constitution *ConstitutionChange

	// Unknown is the raw payload when Cella does not recognise the tag. Showing
	// a delegate the raw JSON is worse than showing them a rendered panel, but
	// it is far better than showing them nothing and letting them assume there
	// was nothing to see.
	Unknown json.RawMessage
}

// Recognised reports whether Cella could decode the payload into something it
// can render.
func (p Payload) Recognised() bool { return p.Kind != "" && p.Unknown == nil }

// --- Treasury withdrawals ---

// Withdrawals is a request to pay out of the treasury.
type Withdrawals struct {
	Recipients []Recipient
	Total      *big.Int // lovelace
	ScriptHash string   // the guardrails script the withdrawal is checked against
}

// Recipient is one payee and the amount they would receive.
type Recipient struct {
	Network    string
	Credential string // key hash, or script hash
	IsScript   bool
	Lovelace   *big.Int
}

// ADA renders lovelace as a decimal ada figure.
func ADA(lovelace *big.Int) string {
	if lovelace == nil {
		return "0"
	}
	whole, frac := new(big.Int).QuoRem(lovelace, big.NewInt(1_000_000), new(big.Int))
	s := addThousands(whole.String())
	if frac.Sign() == 0 {
		return s
	}
	return fmt.Sprintf("%s.%06d", s, frac.Int64())
}

// Percent is this recipient's share of the total, 0–100.
func (r Recipient) Percent(total *big.Int) float64 {
	if total == nil || total.Sign() == 0 || r.Lovelace == nil {
		return 0
	}
	num := new(big.Float).SetInt(r.Lovelace)
	den := new(big.Float).SetInt(total)
	pct, _ := new(big.Float).Quo(num, den).Float64()
	return pct * 100
}

// --- Parameter change ---

// Parameters is a proposed change to protocol parameters.
type Parameters struct {
	Changes    []ParamChange
	ScriptHash string
}

// ParamChange is one parameter and the value proposed for it.
type ParamChange struct {
	Name     string
	Proposed string // rendered; structured values are re-encoded as JSON
}

// --- Hard fork ---

// HardFork is a proposed protocol version bump.
type HardFork struct {
	Major int
	Minor int
}

func (h HardFork) Version() string { return fmt.Sprintf("%d.%d", h.Major, h.Minor) }

// --- Committee update ---

// CommitteeUpdate adds or removes Constitutional Committee seats, and may
// change the quorum threshold.
type CommitteeUpdate struct {
	Added   []Seat
	Removed []string // credentials
	Quorum  *Threshold
}

// Seat is a committee member being added, and the epoch their term ends.
type Seat struct {
	Credential string
	ExpiresAt  int64 // epoch
}

// Threshold is the committee's quorum, as a fraction.
type Threshold struct {
	Numerator   int
	Denominator int
}

func (t Threshold) String() string { return fmt.Sprintf("%d of %d", t.Numerator, t.Denominator) }

// --- New constitution ---

// ConstitutionChange replaces the Constitution itself.
type ConstitutionChange struct {
	AnchorURL  string
	DataHash   string // blake2b-256 of the anchored document
	ScriptHash string // the guardrails script
}

// Decode reads a Koios proposal_description into a Payload.
func Decode(raw json.RawMessage) (Payload, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Payload{}, fmt.Errorf("no payload recorded")
	}

	var envelope struct {
		Tag      string          `json:"tag"`
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Payload{}, fmt.Errorf("payload is not a tagged object: %w", err)
	}
	if envelope.Tag == "" {
		return Payload{}, fmt.Errorf("payload has no tag")
	}

	p := Payload{Kind: Kind(envelope.Tag)}
	var err error

	switch p.Kind {
	case TreasuryWithdrawals:
		p.Withdrawals, err = decodeWithdrawals(envelope.Contents)
	case ParameterChange:
		p.Parameters, err = decodeParameters(envelope.Contents)
	case HardForkInitiation:
		p.HardFork, err = decodeHardFork(envelope.Contents)
	case UpdateCommittee:
		p.Committee, err = decodeCommittee(envelope.Contents)
	case NewConstitution:
		p.Constitution, err = decodeConstitution(envelope.Contents)
	case NoConfidence, InfoAction:
		// Nothing to decode: these carry no parameters a committee needs to read.
		// A no-confidence action's meaning is entirely in the fact of it.
	default:
		p.Unknown = raw
	}
	if err != nil {
		return Payload{}, err
	}
	return p, nil
}

func decodeWithdrawals(c json.RawMessage) (*Withdrawals, error) {
	// contents = [ [ [account, lovelace], ... ], scriptHash ]
	var outer []json.RawMessage
	if err := json.Unmarshal(c, &outer); err != nil || len(outer) == 0 {
		return nil, fmt.Errorf("treasury withdrawal: unexpected shape")
	}

	var pairs [][]json.RawMessage
	if err := json.Unmarshal(outer[0], &pairs); err != nil {
		return nil, fmt.Errorf("treasury withdrawal: recipient list: %w", err)
	}

	w := &Withdrawals{Total: big.NewInt(0)}
	for _, pair := range pairs {
		if len(pair) != 2 {
			return nil, fmt.Errorf("treasury withdrawal: recipient is not [account, amount]")
		}
		var acct struct {
			Network    string `json:"network"`
			Credential struct {
				KeyHash    string `json:"keyHash"`
				ScriptHash string `json:"scriptHash"`
			} `json:"credential"`
		}
		if err := json.Unmarshal(pair[0], &acct); err != nil {
			return nil, fmt.Errorf("treasury withdrawal: account: %w", err)
		}

		// Lovelace amounts exceed float64's exact-integer range, so they are read
		// as arbitrary-precision integers. A treasury withdrawal rounded in the
		// display is a treasury withdrawal misreported.
		amount := new(big.Int)
		if err := json.Unmarshal(pair[1], amount); err != nil {
			return nil, fmt.Errorf("treasury withdrawal: amount: %w", err)
		}

		r := Recipient{Network: acct.Network, Lovelace: amount}
		if acct.Credential.ScriptHash != "" {
			r.Credential, r.IsScript = acct.Credential.ScriptHash, true
		} else {
			r.Credential = acct.Credential.KeyHash
		}
		w.Recipients = append(w.Recipients, r)
		w.Total.Add(w.Total, amount)
	}

	if len(outer) > 1 {
		_ = json.Unmarshal(outer[1], &w.ScriptHash)
	}
	return w, nil
}

func decodeParameters(c json.RawMessage) (*Parameters, error) {
	// contents = [ prevAction, { param: value, ... }, scriptHash ]
	var outer []json.RawMessage
	if err := json.Unmarshal(c, &outer); err != nil || len(outer) < 2 {
		return nil, fmt.Errorf("parameter change: unexpected shape")
	}

	var params map[string]json.RawMessage
	if err := json.Unmarshal(outer[1], &params); err != nil {
		return nil, fmt.Errorf("parameter change: parameters: %w", err)
	}

	p := &Parameters{}
	names := make([]string, 0, len(params))
	for k := range params {
		names = append(names, k)
	}
	sort.Strings(names) // stable order; the map's is not

	for _, name := range names {
		p.Changes = append(p.Changes, ParamChange{
			Name:     name,
			Proposed: renderValue(params[name]),
		})
	}
	if len(outer) > 2 {
		_ = json.Unmarshal(outer[2], &p.ScriptHash)
	}
	return p, nil
}

// renderValue prints a parameter value for a human: bare scalars as themselves,
// structured values as compact JSON.
func renderValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return string(raw)
}

func decodeHardFork(c json.RawMessage) (*HardFork, error) {
	// contents = [ prevAction, {major, minor} ]
	var outer []json.RawMessage
	if err := json.Unmarshal(c, &outer); err != nil || len(outer) == 0 {
		return nil, fmt.Errorf("hard fork: unexpected shape")
	}

	var v struct {
		Major int `json:"major"`
		Minor int `json:"minor"`
	}
	// The version object is the last element; earlier ones are the prior action.
	if err := json.Unmarshal(outer[len(outer)-1], &v); err != nil {
		return nil, fmt.Errorf("hard fork: version: %w", err)
	}
	return &HardFork{Major: v.Major, Minor: v.Minor}, nil
}

func decodeCommittee(c json.RawMessage) (*CommitteeUpdate, error) {
	// contents = [ prevAction, [removed], {credential: expiryEpoch}, {num, den} ]
	var outer []json.RawMessage
	if err := json.Unmarshal(c, &outer); err != nil || len(outer) < 3 {
		return nil, fmt.Errorf("committee update: unexpected shape")
	}

	u := &CommitteeUpdate{}

	// Removed seats. The ledger renders these as credential objects; older
	// records sometimes carry bare strings, so accept either.
	var removed []json.RawMessage
	if err := json.Unmarshal(outer[1], &removed); err == nil {
		for _, r := range removed {
			u.Removed = append(u.Removed, credentialOf(r))
		}
	}

	// Added seats, keyed by credential, valued by the epoch their term ends.
	var added map[string]int64
	if err := json.Unmarshal(outer[2], &added); err != nil {
		return nil, fmt.Errorf("committee update: added seats: %w", err)
	}
	keys := make([]string, 0, len(added))
	for k := range added {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		u.Added = append(u.Added, Seat{Credential: k, ExpiresAt: added[k]})
	}

	if len(outer) > 3 {
		var t Threshold
		if err := json.Unmarshal(outer[3], &struct {
			Numerator   *int `json:"numerator"`
			Denominator *int `json:"denominator"`
		}{&t.Numerator, &t.Denominator}); err == nil && t.Denominator != 0 {
			u.Quorum = &t
		}
	}
	return u, nil
}

// credentialOf pulls a credential hash out of either a bare string or a
// {keyHash|scriptHash} object.
func credentialOf(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var o struct {
		KeyHash    string `json:"keyHash"`
		ScriptHash string `json:"scriptHash"`
	}
	if err := json.Unmarshal(raw, &o); err == nil {
		if o.ScriptHash != "" {
			return o.ScriptHash
		}
		return o.KeyHash
	}
	return string(raw)
}

func decodeConstitution(c json.RawMessage) (*ConstitutionChange, error) {
	// contents = [ prevAction, {anchor: {url, dataHash}, script} ]
	var outer []json.RawMessage
	if err := json.Unmarshal(c, &outer); err != nil || len(outer) < 2 {
		return nil, fmt.Errorf("new constitution: unexpected shape")
	}

	var body struct {
		Anchor struct {
			URL      string `json:"url"`
			DataHash string `json:"dataHash"`
		} `json:"anchor"`
		Script string `json:"script"`
	}
	if err := json.Unmarshal(outer[1], &body); err != nil {
		return nil, fmt.Errorf("new constitution: anchor: %w", err)
	}
	return &ConstitutionChange{
		AnchorURL:  body.Anchor.URL,
		DataHash:   body.Anchor.DataHash,
		ScriptHash: body.Script,
	}, nil
}

// addThousands groups digits for readability. A treasury figure with no
// separators is a figure a delegate has to count on their fingers.
func addThousands(s string) string {
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg, s = true, s[1:]
	}
	n := len(s)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
