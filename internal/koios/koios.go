// Package koios is a minimal client for the parts of the Koios API that Cella
// needs. Koios (https://koios.rest) is a public, decentralized query layer for
// Cardano; no API key is required, though a bearer token raises rate limits.
package koios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to a Koios API instance.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a Koios client for the given base URL (e.g.
// https://api.koios.rest/api/v1). Token may be empty.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// GovernanceAction is one on-chain governance action (a proposal), as returned
// by Koios /proposal_list. Off-chain metadata (CIP-108) arrives in MetaJSON
// when the anchor has been resolved.
type GovernanceAction struct {
	ProposalID string          `json:"proposal_id"`
	TxHash     string          `json:"proposal_tx_hash"`
	Index      int             `json:"proposal_index"`
	Type       string          `json:"proposal_type"`
	BlockTime  int64           `json:"block_time"`
	Expiration *int64          `json:"expiration"`
	MetaURL    string          `json:"meta_url"`
	MetaJSON   json.RawMessage `json:"meta_json"`
}

// Title extracts a human-readable title from the CIP-108 anchored metadata,
// returning "" when none is available.
func (a GovernanceAction) Title() string {
	if len(a.MetaJSON) == 0 {
		return ""
	}
	var m struct {
		Body struct {
			Title string `json:"title"`
		} `json:"body"`
	}
	if err := json.Unmarshal(a.MetaJSON, &m); err == nil {
		return m.Body.Title
	}
	return ""
}

// Abstract extracts the CIP-108 abstract from the anchored metadata, when
// present.
func (a GovernanceAction) Abstract() string {
	if len(a.MetaJSON) == 0 {
		return ""
	}
	var m struct {
		Body struct {
			Abstract string `json:"abstract"`
		} `json:"body"`
	}
	if err := json.Unmarshal(a.MetaJSON, &m); err == nil {
		return m.Body.Abstract
	}
	return ""
}

// GovernanceActions fetches recent governance actions, newest first.
func (c *Client) GovernanceActions(ctx context.Context, limit int) ([]GovernanceAction, error) {
	if limit <= 0 {
		limit = 100
	}
	url := fmt.Sprintf("%s/proposal_list?order=block_time.desc&limit=%d", c.baseURL, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("koios %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	var out []GovernanceAction
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode proposal_list: %w", err)
	}
	return out, nil
}

// Genesis is the network's genesis configuration, as returned by Koios
// /genesis. Cella needs it to turn a governance action's expiration — which the
// chain states as an epoch *number*, not a time — into a wall-clock deadline.
// The values differ per network (mainnet, Preprod and Preview each have their
// own), so they are read from the chain rather than hardcoded.
//
// Koios is inconsistent about types here: systemstart comes back as a JSON
// number and epochlength as a quoted string, so both are read leniently.
type Genesis struct {
	SystemStart flexInt `json:"systemstart"` // unix seconds at which epoch 0 began
	EpochLength flexInt `json:"epochlength"` // seconds per epoch
	NetworkID   string  `json:"networkid"`
}

// flexInt is an integer that JSON may present either bare or quoted.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// GenesisParams are the genesis values Cella actually uses.
type GenesisParams struct {
	SystemStart int64 // unix seconds at which epoch 0 began
	EpochLength int64 // seconds per epoch
}

// Valid reports whether the parameters can be used for epoch arithmetic.
func (p GenesisParams) Valid() bool { return p.SystemStart > 0 && p.EpochLength > 0 }

// EpochStart is the instant epoch n begins.
//
// Byron and Shelley epochs happen to run the same wall-clock length on every
// Cardano network (the slot length shrank as the slots-per-epoch grew), so a
// single multiplication from genesis holds across the era boundary. This is not
// an assumption taken on faith: it reproduces Koios's own slot arithmetic for
// the current epoch exactly.
func (p GenesisParams) EpochStart(n int64) time.Time {
	return time.Unix(p.SystemStart+n*p.EpochLength, 0).UTC()
}

// EpochEnd is the instant epoch n ends, which is when epoch n+1 begins.
func (p GenesisParams) EpochEnd(n int64) time.Time { return p.EpochStart(n + 1) }

// Genesis fetches the network's genesis parameters.
func (c *Client) Genesis(ctx context.Context) (GenesisParams, error) {
	var p GenesisParams

	body, err := c.get(ctx, c.baseURL+"/genesis")
	if err != nil {
		return p, err
	}

	var out []Genesis
	if err := json.Unmarshal(body, &out); err != nil {
		return p, fmt.Errorf("decode genesis: %w", err)
	}
	if len(out) == 0 {
		return p, fmt.Errorf("koios returned no genesis parameters")
	}

	p = GenesisParams{
		SystemStart: int64(out[0].SystemStart),
		EpochLength: int64(out[0].EpochLength),
	}
	if !p.Valid() {
		return GenesisParams{}, fmt.Errorf(
			"genesis parameters are not usable (systemstart=%d epochlength=%d)", p.SystemStart, p.EpochLength)
	}
	return p, nil
}

// get performs an authenticated GET and returns the body.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("koios %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}

// Vote is a single on-chain vote cast on a governance action. VoterRole is one
// of ConstitutionalCommittee, DRep, or SPO; MetaURL anchors the rationale.
type Vote struct {
	VoterRole string `json:"voter_role"`
	VoterID   string `json:"voter_id"`
	Vote      string `json:"vote"`
	MetaURL   string `json:"meta_url"`
	BlockTime int64  `json:"block_time"`
}

// ProposalVotes fetches every on-chain vote cast on a single governance action.
func (c *Client) ProposalVotes(ctx context.Context, proposalID string) ([]Vote, error) {
	u := fmt.Sprintf("%s/proposal_votes?_proposal_id=%s", c.baseURL, url.QueryEscape(proposalID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("koios %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	var out []Vote
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode proposal_votes: %w", err)
	}
	return out, nil
}
