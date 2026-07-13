package server

// rationaleHTML is the committee's rationale authoring page: the reasoning that
// becomes the anchored CIP-136 document, with a live view of the exact bytes
// and the anchor hash they produce.
const rationaleHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Committee rationale — Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#d7ddef; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#6f93ff; --green:#4bbd88; --red:#d9695f; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { padding:30px 6vw 16px; border-bottom:1px solid rgba(201,137,42,.25); }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:22px; }
  header .name b { color:var(--gold); }
  header a.back { color:var(--blue); text-decoration:none; font-size:14px; }
  main { padding:22px 6vw 70px; max-width:900px; }
  h1 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:22px; letter-spacing:.03em; margin:6px 0 6px; line-height:1.3; }
  .sub { color:var(--muted); font-size:14px; }
  .card { background:var(--veil); border:1px solid rgba(201,137,42,.18); border-radius:12px; padding:18px 20px; margin-top:20px; }
  .card h2 { font-family:'Cinzel',serif; color:var(--gold); font-size:13px; letter-spacing:.12em; text-transform:uppercase; margin:0 0 12px; }
  .split { display:flex; align-items:center; gap:14px; flex-wrap:wrap; font-size:15px; }
  .split .y { color:var(--green); } .split .n { color:var(--red); } .split .a { color:var(--muted); } .split .dn { color:var(--muted); font-style:italic; }
  .pill { font-family:'Cinzel',serif; font-size:12px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; padding:4px 14px; border-radius:999px; border:1px solid; }
  .pill.Yes { color:var(--green); border-color:rgba(75,189,136,.5); }
  .pill.No { color:var(--red); border-color:rgba(217,105,95,.5); }
  .pill.Abstain { color:var(--muted); border-color:rgba(139,147,184,.4); }
  .note { color:var(--muted); font-size:13.5px; line-height:1.55; margin-top:10px; }
  label.f { display:block; margin-top:16px; }
  label.f .lt { font-family:'Cinzel',serif; color:var(--ivory); font-size:13px; letter-spacing:.04em; }
  label.f .lh { color:var(--muted); font-size:13px; margin:3px 0 7px; line-height:1.5; }
  label.f .req { color:var(--goldb); font-size:11px; font-family:'Cinzel',serif; letter-spacing:.08em; text-transform:uppercase; margin-left:7px; }
  textarea, input[type=text] { width:100%; background:var(--forum); border:1px solid rgba(201,137,42,.25); border-radius:10px; color:var(--body); font-family:'EB Garamond',Georgia,serif; font-size:15px; padding:11px 13px; resize:vertical; }
  textarea:focus, input[type=text]:focus { outline:none; border-color:rgba(245,210,122,.6); }
  textarea.sm { min-height:64px; } textarea.lg { min-height:150px; }
  .count { color:var(--muted); font-size:12px; margin-top:4px; text-align:right; }
  .count.over { color:var(--red); }
  .save { margin-top:18px; font-family:'Cinzel',serif; font-size:12px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--forum); background:linear-gradient(180deg,var(--goldb),var(--gold)); border:0; border-radius:10px; padding:12px 24px; cursor:pointer; }
  .status { margin-top:14px; padding:11px 14px; border-radius:10px; font-size:14px; }
  .status.ok { border:1px solid rgba(75,189,136,.4); background:rgba(75,189,136,.08); color:var(--green); }
  .status.bad { border:1px solid rgba(245,210,122,.45); background:rgba(245,210,122,.07); color:var(--goldb); }
  .anchor { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:12.5px; color:var(--goldb); word-break:break-all; background:var(--forum); border-radius:8px; padding:9px 12px; margin-top:8px; }
  pre.jsonld { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11.5px; line-height:1.5; color:var(--body); background:var(--forum); border-radius:9px; padding:13px 15px; overflow:auto; max-height:340px; margin:0; }
  details summary { cursor:pointer; color:var(--goldb); font-family:'Cinzel',serif; font-size:12px; letter-spacing:.06em; text-transform:uppercase; }
  .dl { display:inline-block; margin-top:12px; font-family:'Cinzel',serif; font-size:12px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--forum); background:linear-gradient(180deg,var(--goldb),var(--gold)); text-decoration:none; border-radius:10px; padding:11px 20px; }
  .dl.off { background:none; border:1px solid rgba(139,147,184,.35); color:var(--muted); cursor:not-allowed; }
  .authors { color:var(--body); font-size:14.5px; line-height:1.6; }
  .authors .none { color:var(--muted); font-style:italic; }
  footer { padding:20px 6vw; color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="name">CE<b>LL</b>A</div>
  <a class="back" href="/action/{{.Slug}}">← Back to the action</a>
</header>
<main>
  <div class="sub">The committee's rationale</div>
  <h1>{{if .Title}}{{.Title}}{{else}}Governance action{{end}}</h1>

  <div class="card">
    <h2>What the chamber decided</h2>
    <div class="split">
      <span class="pill {{.Decision}}">{{.Decision}}</span>
      <span><span class="y">{{.Tally.Yes}} constitutional</span> · <span class="n">{{.Tally.No}} unconstitutional</span> · <span class="a">{{.Tally.Abstain}} abstain</span> · <span class="dn">{{.Tally.DidNotVote}} did not vote</span></span>
    </div>
    <div class="note">These counts become the CIP-136 <code>internalVote</code> block, which is how a multi-member committee shows the chain that its single vote came from a real internal split. A delegate voting Yes judges the action constitutional; No, unconstitutional.</div>
  </div>

  <form method="post" action="/rationale/{{.Slug}}">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    <div class="card">
      <h2>Author the rationale</h2>

      <label class="f">
        <span class="lt">Summary<span class="req">required · max {{.SummaryMax}}</span></span>
        <span class="lh">A single plain sentence stating the committee's finding. Plain text — no markdown.</span>
        <textarea class="sm" name="summary" id="summary" maxlength="{{.SummaryMax}}" placeholder="The committee finds this action constitutional…">{{.Summary}}</textarea>
        <div class="count" id="count"></div>
      </label>

      <label class="f">
        <span class="lt">Rationale statement<span class="req">required</span></span>
        <span class="lh">The committee's full reasoning, citing the articles it judged against. Markdown is supported.</span>
        <textarea class="lg" name="statement" placeholder="The action is assessed against Article …">{{.Statement}}</textarea>
      </label>

      <label class="f">
        <span class="lt">Precedent</span>
        <span class="lh">Prior actions or committee decisions that bear on this one.</span>
        <textarea class="sm" name="precedent">{{.Precedent}}</textarea>
      </label>

      <label class="f">
        <span class="lt">Counterargument</span>
        <span class="lh">The strongest case against the committee's finding, stated fairly.</span>
        <textarea class="sm" name="counterargument">{{.Counterargument}}</textarea>
      </label>

      <label class="f">
        <span class="lt">Conclusion</span>
        <span class="lh">How the committee resolved the question. Plain text — no markdown.</span>
        <textarea class="sm" name="conclusion">{{.Conclusion}}</textarea>
      </label>

      <button type="submit" class="save">{{if .Authored}}Update the rationale{{else}}Record the rationale{{end}}</button>
      {{if .AuthoredBy}}<div class="note">Last edited by {{.AuthoredBy}}.</div>{{end}}
    </div>
  </form>

  <div class="card">
    <h2>Authors</h2>
    <div class="authors">
      {{if .Authors}}
        {{range $i, $a := .Authors}}{{if $i}}, {{end}}{{$a}}{{end}}
        <div class="note">Every delegate who recorded a position on this action is named as an author. Each signs the document with their own key at witnessing time — Cella emits the witness slots unfilled.</div>
      {{else}}
        <span class="none">No delegate has recorded a position yet, so the document has no authors.</span>
      {{end}}
    </div>
  </div>

  <div class="card">
    <h2>The anchored document (CIP-136)</h2>
    {{if .Valid}}
      <div class="status ok">&#10003; Ready to anchor.</div>
      <div class="note">Anchor hash — the blake2b-256 of the exact bytes below, and the value submitted on-chain with the vote:</div>
      <div class="anchor">{{.AnchorHash}}</div>
      <div class="note"><code>cardano-cli hash anchor-data --file-text rationale-{{.Slug}}.jsonld</code> prints this same hash for the downloaded file.</div>
      <a class="dl" href="/rationale/{{.Slug}}.jsonld">Download the .jsonld &#8595;</a>
    {{else}}
      <div class="status bad">Not ready to anchor — {{.Problem}}.</div>
      <div class="note">The anchor hash appears once the document satisfies CIP-136. A hash over an incomplete rationale would look submittable but would not be.</div>
      <span class="dl off">Download the .jsonld</span>
    {{end}}
    <div style="margin-top:16px;">
      <details>
        <summary>Show the document</summary>
        <pre class="jsonld">{{.JSONLD}}</pre>
      </details>
    </div>
  </div>
</main>
<footer>Cella · CIP-136 committee vote rationale · Apache-2.0</footer>

<script>
(function () {
  var box = document.getElementById('summary'), out = document.getElementById('count'), max = {{.SummaryMax}};
  if (!box || !out) return;
  function tick() {
    var n = Array.from(box.value).length;   // count characters, not UTF-16 units
    out.textContent = n + ' / ' + max;
    out.classList.toggle('over', n > max);
  }
  box.addEventListener('input', tick);
  tick();
})();
</script>
</body>
</html>`
