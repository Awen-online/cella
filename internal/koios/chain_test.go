package koios

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeKoios serves a canned body for any request, and records what it was asked.
func fakeKoios(t *testing.T, status int, body string) (*Client, *http.Request) {
	t.Helper()
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, ""), got
}

// Koios is inconsistent here: systemstart comes back as a bare JSON number,
// epochlength as a quoted string. A decoder that insists on one shape breaks on
// the other, and the whole deadline countdown depends on both.
func TestGenesisReadsMixedTypes(t *testing.T) {
	c, _ := fakeKoios(t, http.StatusOK, `[{
		"networkid": "Mainnet",
		"systemstart": 1506203091,
		"epochlength": "432000"
	}]`)

	got, err := c.Genesis(context.Background())
	if err != nil {
		t.Fatalf("Genesis: %v", err)
	}
	if got.SystemStart != 1506203091 {
		t.Errorf("SystemStart = %d, want 1506203091 (a bare JSON number)", got.SystemStart)
	}
	if got.EpochLength != 432000 {
		t.Errorf("EpochLength = %d, want 432000 (a quoted JSON string)", got.EpochLength)
	}
	if !got.Valid() {
		t.Error("mainnet genesis parameters did not validate")
	}
}

// Genesis parameters that cannot support epoch arithmetic must be an error, not
// a zero value that silently makes every action look expired in 1970 — which is
// exactly the bug this whole path exists to fix.
func TestGenesisRejectsUnusableParameters(t *testing.T) {
	cases := map[string]string{
		"empty array":      `[]`,
		"zero systemstart": `[{"systemstart": 0, "epochlength": "432000"}]`,
		"zero epochlength": `[{"systemstart": 1506203091, "epochlength": "0"}]`,
		"missing fields":   `[{"networkid":"Mainnet"}]`,
		"not an array":     `{"systemstart": 1}`,
		"nonsense":         `not json`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := fakeKoios(t, http.StatusOK, body)
			got, err := c.Genesis(context.Background())
			if err == nil {
				t.Errorf("Genesis succeeded with %+v; want an error", got)
			}
			if got.Valid() {
				t.Error("an unusable GenesisParams reported itself Valid()")
			}
		})
	}
}

func TestGenesisPropagatesHTTPErrors(t *testing.T) {
	c, _ := fakeKoios(t, http.StatusInternalServerError, `{"error":"boom"}`)
	if _, err := c.Genesis(context.Background()); err == nil {
		t.Error("a 500 from Koios was not reported as an error")
	}
}

// The epoch arithmetic these parameters drive.
func TestEpochBounds(t *testing.T) {
	p := GenesisParams{SystemStart: 1506203091, EpochLength: 432000}

	// Cross-checked against Koios's own slot numbers for mainnet epoch 642.
	if got := p.EpochStart(642).Unix(); got != 1783547091 {
		t.Errorf("EpochStart(642) = %d, want 1783547091", got)
	}
	if !p.EpochEnd(642).Equal(p.EpochStart(643)) {
		t.Error("epochs are not contiguous")
	}
	if d := p.EpochEnd(642).Sub(p.EpochStart(642)); d != 5*24*time.Hour {
		t.Errorf("a mainnet epoch is %s, want 120h", d)
	}
}

// The hot NFT lives alone at its script address, and its datum is what sets
// quorum. Reading the wrong UTxO would mean reading the wrong voting group.
func TestHotNFTDatum(t *testing.T) {
	const datum = "9fd8799f581cc6731b9c6de6bf11d91f08099953cb393505806ff522e5cc3a7574ab" +
		"5820e50384c655f9a33cabf64e41df7282e765a242aef182130f1db01bce8859e0aaffff"

	c, _ := fakeKoios(t, http.StatusOK, `[{
		"tx_hash": "abc",
		"is_spent": false,
		"inline_datum": {"bytes": "`+datum+`"}
	}]`)

	got, err := c.HotNFTDatum(context.Background(), "addr1w_hot_nft")
	if err != nil {
		t.Fatalf("HotNFTDatum: %v", err)
	}
	if got != datum {
		t.Errorf("datum = %s, want %s", got, datum)
	}
}

func TestHotNFTDatumRefusesAmbiguity(t *testing.T) {
	cases := map[string]struct {
		body      string
		wantInErr string
	}{
		// Nothing there: the configured address is probably not the hot NFT's.
		"no utxos": {`[]`, "no unspent UTxO"},

		// A UTxO with no inline datum tells us nothing about the voting group.
		"no inline datum": {`[{"tx_hash":"a","inline_datum":null}]`, "no unspent UTxO"},

		// Spent UTxOs are history, not the current voting group.
		"only spent": {`[{"tx_hash":"a","is_spent":true,"inline_datum":{"bytes":"9fff"}}]`, "no unspent UTxO"},

		// Two candidate datums: picking one and hoping would be guessing at
		// quorum, which is the one thing this must never do.
		"two datums": {
			`[{"tx_hash":"a","inline_datum":{"bytes":"9fff"}},
			  {"tx_hash":"b","inline_datum":{"bytes":"80"}}]`,
			"should be alone at its address",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := fakeKoios(t, http.StatusOK, tc.body)
			got, err := c.HotNFTDatum(context.Background(), "addr1w_hot_nft")
			if err == nil {
				t.Fatalf("HotNFTDatum returned %q; want an error", got)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantInErr)
			}
		})
	}
}

// The address must actually reach Koios in the request body, or we would be
// reading whatever UTxOs Koios felt like returning.
func TestHotNFTDatumQueriesTheGivenAddress(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, _ = c.HotNFTDatum(context.Background(), "addr1w_the_hot_nft")

	if !strings.Contains(body, "addr1w_the_hot_nft") {
		t.Errorf("the request body did not carry the address: %s", body)
	}
	if !strings.Contains(body, "_addresses") {
		t.Errorf("the request body did not use Koios's _addresses parameter: %s", body)
	}
}
