package server

import (
	"net/http"
	"strings"

	"github.com/Awen-online/cella/internal/rationale"
	"github.com/Awen-online/cella/internal/store"
)

// submitView drives the on-chain submission flow for one action.
type submitView struct {
	store.ActionRow
	Decision string   // Yes | No | Abstain — the committee's resolved vote
	Tally    tally    // how the delegates split to get there
	Members  []Member // CC cold-key co-signers (the body's delegates)

	// The rationale that is anchored with the vote. AnchorHash is real: it is
	// the blake2b-256 of the exact bytes served at /rationale/{slug}.jsonld, and
	// the value `cardano-cli hash anchor-data --file-text` prints for that file.
	// Ready is false when no anchorable rationale has been authored yet, in
	// which case there is nothing to submit.
	Ready      bool
	Problem    string
	AnchorHash string
}

// handleSubmit renders the on-chain submission flow: anchor the rationale,
// compose the committee vote, build the transaction, collect the CC cold-key
// signatures, and submit. Modelled on the credential-manager / hot-NFT multisig
// runbook (orchestrator-cli + cardano-cli conway).
//
// The rationale and its anchor hash are real artifacts. The transaction is not:
// nothing is broadcast, because that requires the committee's cold keys.
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/submit/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	a, ok, err := s.db.ActionBySlug(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	t, _, err := s.tallyFor(a.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	v := submitView{
		ActionRow: a,
		Decision:  t.Decision(),
		Tally:     t,
		Members:   demoBody.Members,
	}

	// A committee submits its reasoning with its vote. Without an anchorable
	// rationale there is no anchor hash, and so nothing to compose a vote around.
	doc, _, err := s.docFor(a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := doc.Validate(); err != nil {
		v.Problem = err.Error()
	} else {
		jsonld, err := doc.JSONLD()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		v.AnchorHash = rationale.AnchorHash(jsonld)
		v.Ready = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.stpl.Execute(w, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// submitHTML is the mock on-chain submission wizard.
const submitHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Submit on-chain — Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#d7ddef; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --goldd:#7A5418; --blue:#6f93ff; --green:#4bbd88; --red:#d9695f; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { padding:30px 6vw 16px; border-bottom:1px solid rgba(201,137,42,.25); }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:22px; }
  header .name b { color:var(--gold); }
  header a.back { color:var(--blue); text-decoration:none; font-size:14px; }
  main { padding:22px 6vw 70px; max-width:820px; }
  h1 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:22px; letter-spacing:.03em; margin:6px 0 6px; }
  .sub { color:var(--muted); font-size:14px; margin-bottom:6px; }
  .decision { margin:16px 0 6px; font-size:16px; }
  .decision .pill { font-family:'Cinzel',serif; font-size:12px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; padding:4px 14px; border-radius:999px; border:1px solid; }
  .decision .pill.Yes { color:var(--green); border-color:rgba(75,189,136,.5); }
  .decision .pill.No { color:var(--red); border-color:rgba(217,105,95,.5); }
  .decision .pill.Abstain { color:var(--muted); border-color:rgba(139,147,184,.4); }
  .demo-banner { margin:14px 0 20px; padding:10px 14px; border:1px dashed rgba(245,210,122,.5); border-radius:10px; color:var(--goldb); font-size:13.5px; }
  .steps { display:flex; flex-direction:column; gap:12px; margin-top:8px; }
  .st { display:grid; grid-template-columns:34px 1fr; gap:14px; align-items:start; background:var(--veil); border:1px solid rgba(201,137,42,.18); border-radius:12px; padding:14px 16px; opacity:.55; transition:opacity .3s; }
  .st.active { opacity:1; border-color:rgba(245,210,122,.4); }
  .st.done { opacity:1; }
  .st .dot { width:26px; height:26px; border-radius:50%; border:1.5px solid var(--gold); color:var(--gold); font-family:'Cinzel',serif; font-size:12px; display:flex; align-items:center; justify-content:center; }
  .st.done .dot { background:var(--green); border-color:var(--green); color:var(--forum); }
  .st .t { font-family:'Cinzel',serif; color:var(--ivory); font-size:14px; letter-spacing:.03em; }
  .st .d { color:var(--muted); font-size:13.5px; margin-top:3px; }
  .st .cmd { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11.5px; color:var(--goldb); margin-top:7px; background:var(--forum); border-radius:7px; padding:7px 10px; overflow-x:auto; white-space:pre; }
  .signers { display:flex; flex-wrap:wrap; gap:8px; margin-top:9px; }
  .signer { font-size:12px; border:1px solid rgba(201,137,42,.3); border-radius:999px; padding:3px 11px; color:var(--muted); }
  .signer.signed { color:var(--green); border-color:rgba(75,189,136,.5); }
  .decision .split { color:var(--muted); font-size:13.5px; margin-left:8px; }
  .decision .split .y { color:var(--green); } .decision .split .n { color:var(--red); } .decision .split .a { color:var(--muted); }
  .hashline { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11.5px; color:var(--green); margin-top:6px; word-break:break-all; }
  .rlink { color:var(--blue); text-decoration:none; font-size:13px; }
  .muted { color:var(--muted); font-style:italic; }
  .blocked { margin:16px 0 20px; padding:16px 18px; border:1px solid rgba(217,105,95,.4); border-radius:12px; background:rgba(217,105,95,.07); }
  .blocked h3 { font-family:'Cinzel',serif; color:var(--red); margin:0 0 8px; font-size:15px; }
  .blocked .go-link { display:inline-block; margin-top:12px; font-family:'Cinzel',serif; font-size:12px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--goldb); text-decoration:none; border:1px solid rgba(245,210,122,.45); border-radius:10px; padding:9px 18px; }
  .actions { margin-top:26px; }
  .go { font-family:'Cinzel',serif; font-size:14px; letter-spacing:.1em; text-transform:uppercase; font-weight:700; color:var(--forum); background:linear-gradient(180deg,var(--goldb),var(--gold)); border:0; border-radius:12px; padding:15px 30px; cursor:pointer; }
  .go:disabled { opacity:.6; cursor:default; }
  .result { margin-top:20px; display:none; padding:16px 18px; border:1px solid rgba(75,189,136,.4); border-radius:12px; background:rgba(75,189,136,.08); }
  .result h3 { font-family:'Cinzel',serif; color:var(--green); margin:0 0 8px; font-size:15px; }
  .result .tx { font-family:'JetBrains Mono',monospace; font-size:12px; color:var(--goldb); word-break:break-all; }
  .result .note { color:var(--muted); font-size:12.5px; margin-top:8px; font-style:italic; }
  footer { padding:20px 6vw; color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="name">CE<b>LL</b>A</div>
  <a class="back" href="/action/{{.Slug}}">← Back to the action</a>
</header>
<main>
  <div class="sub">Submit the committee's vote on-chain</div>
  <h1>{{if .Title}}{{.Title}}{{else}}Governance action{{end}}</h1>
  <div class="decision">Committee decision from the chamber: <span class="pill {{.Decision}}">{{.Decision}}</span>
    <span class="split">from <span class="y">{{.Tally.Yes}} Yes</span> · <span class="n">{{.Tally.No}} No</span> · <span class="a">{{.Tally.Abstain}} Abstain</span>{{if .Tally.DidNotVote}} · {{.Tally.DidNotVote}} awaiting{{end}}</span>
  </div>
  <div class="demo-banner"><b>The rationale and its anchor hash below are real</b> — the same bytes and the same hash a committee would anchor. The transaction is not: this walks the credential-manager / hot-NFT multisig flow without broadcasting, because that requires the CC cold keys.</div>

  {{if not .Ready}}
  <div class="blocked">
    <h3>Nothing to submit yet</h3>
    <div>The committee votes with its reasoning attached, and this action has no anchorable rationale — {{.Problem}}.</div>
    <a class="go-link" href="/rationale/{{.Slug}}">Author the committee rationale &#8594;</a>
  </div>
  {{end}}

  <div class="steps" id="steps">
    <div class="st" data-i="0">
      <div class="dot">1</div>
      <div>
        <div class="t">Anchor the rationale (CIP-136)</div>
        <div class="d">Publish the committee's rationale and hash the anchor.</div>
        {{if .Ready}}
        <div class="cmd">cardano-cli hash anchor-data --file-text rationale-{{.Slug}}.jsonld</div>
        <div class="hashline">{{.AnchorHash}}</div>
        <div class="d"><a class="rlink" href="/rationale/{{.Slug}}.jsonld">Download the document this hashes &#8595;</a></div>
        {{else}}
        <div class="d muted">No rationale authored yet.</div>
        {{end}}
      </div>
    </div>
    <div class="st" data-i="1">
      <div class="dot">2</div>
      <div>
        <div class="t">Compose the committee vote</div>
        <div class="d">Build the {{.Decision}} vote against the hot-NFT credential via the orchestrator.</div>
        <div class="cmd">orchestrator-cli vote --hot-credential-script-file hot/credential.plutus \
  --governance-action-tx-id {{.TxHash}} --governance-action-index {{.Idx}} --vote {{.Decision}}</div>
      </div>
    </div>
    <div class="st" data-i="2">
      <div class="dot">3</div>
      <div>
        <div class="t">Build the transaction</div>
        <div class="d">Assemble the tx requiring the committee's cold-key signers.</div>
        <div class="cmd">cardano-cli conway transaction build --tx-in $HOT_NFT_UTXO --required-signer-hash &lt;cc-cold-1&gt; ...</div>
      </div>
    </div>
    <div class="st" data-i="3">
      <div class="dot">4</div>
      <div>
        <div class="t">Collect CC cold-key signatures</div>
        <div class="d">Each delegate co-signs with their cold key (multisig).</div>
        <div class="signers" id="signers">
          {{range .Members}}<span class="signer" data-name="{{.Name}}">{{.Name}}</span>{{end}}
        </div>
      </div>
    </div>
    <div class="st" data-i="4">
      <div class="dot">5</div>
      <div>
        <div class="t">Submit on-chain</div>
        <div class="d">Broadcast the assembled, fully-witnessed transaction.</div>
        <div class="cmd">cardano-cli conway transaction submit --tx-file tx.signed</div>
      </div>
    </div>
  </div>

  <div class="actions">
    <button class="go" id="go" {{if not .Ready}}disabled{{end}}>Sign with CC cold keys &amp; submit (demo)</button>
  </div>

  <div class="result" id="result">
    <h3>&#10003; Vote submitted (demo)</h3>
    <div>Transaction: <span class="tx" id="txid"></span></div>
    <div class="note">Nothing was broadcast to the network. In a live instance this is where the fully-witnessed transaction goes on-chain.</div>
  </div>
</main>
<footer>Cella · mock on-chain submission · no transaction is broadcast · Apache-2.0</footer>

<script>
(function () {
  var go = document.getElementById('go');
  var steps = Array.prototype.slice.call(document.querySelectorAll('.st'));
  var signers = Array.prototype.slice.call(document.querySelectorAll('#signers .signer'));
  var result = document.getElementById('result');

  function wait(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }
  function hex(n) { var s = ''; var c = '0123456789abcdef'; for (var i = 0; i < n; i++) s += c[Math.floor(Math.random() * 16)]; return s; }

  go.addEventListener('click', async function () {
    go.disabled = true; result.style.display = 'none';
    steps.forEach(function (s) { s.classList.remove('active', 'done'); });
    signers.forEach(function (s) { s.classList.remove('signed'); });

    for (var i = 0; i < steps.length; i++) {
      steps[i].classList.add('active');
      steps[i].scrollIntoView({ behavior: 'smooth', block: 'center' });
      if (i === 3) { // cold-key signatures
        for (var j = 0; j < signers.length; j++) { await wait(520); signers[j].classList.add('signed'); }
      } else {
        await wait(950);
      }
      steps[i].classList.remove('active'); steps[i].classList.add('done');
    }
    document.getElementById('txid').textContent = hex(64);
    result.style.display = 'block';
    result.scrollIntoView({ behavior: 'smooth', block: 'center' });
    go.disabled = false; go.textContent = 'Run the flow again (demo)';
  });
})();
</script>
</body>
</html>`
