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
