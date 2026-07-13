package server

import (
	"html/template"
	"net/http"

	"github.com/Awen-online/cella/internal/constitution"
)

type constPage struct {
	Body     template.HTML
	TOC      []constitution.Entry
	Active   string
	Label    string
	Versions []constitution.Version

	// Focus is a heading anchor to scroll to, arriving from a governance
	// action's constitutional-alignment links.
	Focus string
}

// handleConstitution serves the Cardano Constitution, the yardstick against
// which the committee judges every governance action.
//
// It is a working document, not a monument: it needs a table of contents, an
// anchor on every article, and a search that can find a clause a delegate half
// remembers. A Constitution nobody can navigate is a Constitution nobody cites.
func (s *Server) handleConstitution(w http.ResponseWriter, r *http.Request) {
	body, toc, v, err := constitution.HTML(r.URL.Query().Get("v"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page := constPage{
		Body:     body,
		TOC:      toc,
		Active:   v.Key,
		Label:    v.Label,
		Versions: constitution.Versions,
		Focus:    r.URL.Query().Get("focus"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.ctpl.Execute(w, page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// constHTML is the Constitution reader: versioned, navigable, searchable.
const constHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cardano Constitution ({{.Label}}) — Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#d7ddef; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#6f93ff; --green:#4bbd88; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { padding:26px 4vw 14px; border-bottom:1px solid rgba(201,137,42,.25); }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:22px; }
  header .name b { color:var(--gold); }
  header a.back { color:var(--blue); text-decoration:none; font-size:14px; }

  .bar { position:sticky; top:0; z-index:20; background:rgba(10,14,39,.96); backdrop-filter:blur(6px);
         padding:12px 4vw; display:flex; align-items:center; gap:12px; flex-wrap:wrap;
         border-bottom:1px solid rgba(201,137,42,.18); }
  .bar .lbl { font-family:'Cinzel',serif; color:var(--gold); font-size:11px; letter-spacing:.12em; text-transform:uppercase; }
  .bar a.ver { font-size:13px; text-decoration:none; color:var(--muted); border:1px solid rgba(201,137,42,.3); border-radius:999px; padding:3px 12px; }
  .bar a.ver.on { color:var(--forum); background:var(--goldb); border-color:var(--goldb); font-weight:600; }

  .find { margin-left:auto; display:inline-flex; align-items:center; gap:7px; background:var(--veil);
          border:1px solid rgba(201,137,42,.3); border-radius:999px; padding:3px 6px 3px 14px; }
  .find input { border:0; outline:0; background:transparent; color:var(--body); font-family:'EB Garamond',Georgia,serif;
                font-size:14px; min-width:230px; padding:5px 0; }
  .find input::placeholder { color:var(--muted); }
  .find .count { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11px; color:var(--muted); min-width:44px; text-align:right; }
  .find button { background:transparent; border:1px solid rgba(201,137,42,.3); border-radius:50%; width:24px; height:24px;
                 cursor:pointer; color:var(--muted); font-size:12px; line-height:1; }
  .find button:hover { color:var(--goldb); border-color:var(--goldb); }

  .wrap { display:grid; grid-template-columns:290px 1fr; gap:32px; padding:20px 4vw 70px; max-width:1400px; }
  @media (max-width:960px) { .wrap { grid-template-columns:1fr; } nav.toc { position:static; max-height:none; } }

  nav.toc { position:sticky; top:74px; align-self:start; max-height:calc(100vh - 100px); overflow-y:auto; padding-right:8px; }
  nav.toc .tlbl { font-family:'Cinzel',serif; color:var(--gold); font-size:10.5px; letter-spacing:.14em; text-transform:uppercase; margin-bottom:10px; }
  nav.toc a { display:block; color:var(--muted); text-decoration:none; font-size:13.5px; line-height:1.35; padding:5px 9px; border-radius:7px; border-left:2px solid transparent; }
  nav.toc a:hover { color:var(--ivory); background:rgba(245,210,122,.07); }
  nav.toc a.l2 { color:var(--body); font-family:'Cinzel',serif; font-size:11.5px; letter-spacing:.04em; margin-top:9px; }
  nav.toc a.l3 { padding-left:20px; font-size:13px; }
  nav.toc a.here { color:var(--goldb); border-left-color:var(--gold); background:rgba(245,210,122,.09); }

  article.doc { max-width:78ch; font-size:17px; line-height:1.65; }
  article.doc h1 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:800; font-size:25px; letter-spacing:.03em; margin:8px 0 18px; }
  article.doc h2 { font-family:'Cinzel',serif; color:var(--goldb); font-weight:700; font-size:19px; letter-spacing:.04em; text-transform:uppercase; margin:34px 0 8px; padding-bottom:6px; border-bottom:1px solid rgba(201,137,42,.2); }
  article.doc h3 { font-family:'Cinzel',serif; color:var(--gold); font-weight:700; font-size:15.5px; margin:24px 0 6px; }
  article.doc h1,article.doc h2,article.doc h3 { scroll-margin-top:86px; position:relative; }
  article.doc h2:hover .permalink, article.doc h3:hover .permalink { opacity:1; }
  .permalink { position:absolute; left:-22px; opacity:0; color:var(--gold); text-decoration:none; font-size:15px; transition:opacity .15s; }
  article.doc p { margin:11px 0; }
  article.doc ol, article.doc ul { padding-left:26px; margin:10px 0; }
  article.doc li { margin:6px 0; }
  article.doc a { color:var(--blue); }
  article.doc hr { border:0; border-top:1px solid rgba(201,137,42,.2); margin:28px 0; }
  article.doc blockquote { border-left:3px solid var(--gold); margin:14px 0; padding:2px 16px; color:var(--muted); }
  article.doc table { border-collapse:collapse; width:100%; margin:14px 0; }
  article.doc th, article.doc td { border:1px solid rgba(201,137,42,.2); padding:8px 10px; text-align:left; font-size:15px; }
  article.doc h2:target, article.doc h3:target { background:rgba(245,210,122,.1); border-radius:6px; padding-left:8px; margin-left:-8px; }

  mark.hit { background:rgba(245,210,122,.28); color:var(--ivory); border-radius:3px; padding:0 1px; }
  mark.hit.on { background:var(--goldb); color:var(--forum); }

  footer { padding:20px 4vw; color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="name">CE<b>LL</b>A</div>
  <a class="back" href="/">← Governance actions</a>
</header>

<div class="bar">
  <span class="lbl">Revision</span>
  {{range .Versions}}<a class="ver {{if eq .Key $.Active}}on{{end}}" href="/constitution?v={{.Key}}">{{.Label}}</a>{{end}}
  <label class="find">
    <input type="search" id="q" placeholder="Search the Constitution…" autocomplete="off" spellcheck="false" aria-label="Search the Constitution">
    <span class="count" id="qn"></span>
    <button type="button" id="qp" title="Previous match (Shift+Enter)" aria-label="Previous match">&#8593;</button>
    <button type="button" id="qnx" title="Next match (Enter)" aria-label="Next match">&#8595;</button>
  </label>
</div>

<div class="wrap">
  <nav class="toc" aria-label="Table of contents">
    <div class="tlbl">Contents</div>
    {{range .TOC}}<a class="l{{.Level}}" href="#{{.ID}}">{{.Text}}</a>{{end}}
  </nav>
  <article class="doc" id="doc">{{.Body}}</article>
</div>

<footer>The Cardano Constitution is the yardstick for every Constitutional Committee vote. Source: <a href="https://github.com/IntersectMBO/cardano-constitution" rel="noopener" style="color:var(--blue)">IntersectMBO/cardano-constitution</a> · Cella · Apache-2.0</footer>

<script>
(function () {
  var doc = document.getElementById('doc');
  if (!doc) return;

  // A permalink on every article and section. A rationale that says "contrary
  // to Article III" should be able to hand the reader Article III.
  doc.querySelectorAll('h2[id], h3[id]').forEach(function (h) {
    var a = document.createElement('a');
    a.className = 'permalink';
    a.href = '#' + h.id;
    a.textContent = '#';
    a.setAttribute('aria-label', 'Link to this section');
    h.insertBefore(a, h.firstChild);
  });

  // Highlight the table-of-contents entry for whatever is on screen, so a
  // delegate always knows where in the document they are.
  var links = {}, heads = [];
  document.querySelectorAll('nav.toc a').forEach(function (a) {
    links[decodeURIComponent(a.getAttribute('href').slice(1))] = a;
  });
  doc.querySelectorAll('h2[id], h3[id]').forEach(function (h) { if (links[h.id]) heads.push(h); });

  var here = null;
  function spy() {
    var best = null;
    for (var i = 0; i < heads.length; i++) {
      if (heads[i].getBoundingClientRect().top <= 120) best = heads[i]; else break;
    }
    if (!best || best === here) return;
    if (here && links[here.id]) links[here.id].classList.remove('here');
    here = best;
    links[here.id].classList.add('here');
  }
  var ticking = false;
  window.addEventListener('scroll', function () {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(function () { spy(); ticking = false; });
  }, { passive: true });
  spy();

  // In-page search. Walks text nodes and wraps matches, so it finds a phrase
  // wherever it sits in the markup — then lets you step through the hits.
  var q = document.getElementById('q'), qn = document.getElementById('qn');
  var pristine = doc.innerHTML;
  var hits = [], at = -1, timer = null;

  function esc(s) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }

  function clear() {
    doc.innerHTML = pristine;
    hits = []; at = -1; qn.textContent = '';
    // The permalinks and scrollspy were attached to nodes we just replaced.
    doc.querySelectorAll('h2[id], h3[id]').forEach(function (h) {
      var a = document.createElement('a');
      a.className = 'permalink'; a.href = '#' + h.id; a.textContent = '#';
      h.insertBefore(a, h.firstChild);
    });
    heads = [];
    doc.querySelectorAll('h2[id], h3[id]').forEach(function (h) { if (links[h.id]) heads.push(h); });
    here = null; spy();
  }

  function run(term) {
    clear();
    if (term.length < 2) return;

    var re = new RegExp(esc(term), 'gi');
    var walker = document.createTreeWalker(doc, NodeFilter.SHOW_TEXT, null);
    var texts = [], n;
    while ((n = walker.nextNode())) texts.push(n);

    texts.forEach(function (node) {
      var s = node.nodeValue;
      if (!re.test(s)) return;
      re.lastIndex = 0;

      var frag = document.createDocumentFragment(), last = 0, m;
      while ((m = re.exec(s))) {
        if (m.index > last) frag.appendChild(document.createTextNode(s.slice(last, m.index)));
        var mark = document.createElement('mark');
        mark.className = 'hit';
        mark.textContent = m[0];
        frag.appendChild(mark);
        last = m.index + m[0].length;
        if (m[0].length === 0) re.lastIndex++;   // guard against a zero-width loop
      }
      if (last < s.length) frag.appendChild(document.createTextNode(s.slice(last)));
      node.parentNode.replaceChild(frag, node);
    });

    hits = Array.prototype.slice.call(doc.querySelectorAll('mark.hit'));
    if (!hits.length) { qn.textContent = '0'; return; }
    go(0);
  }

  function go(i) {
    if (!hits.length) return;
    if (at >= 0 && hits[at]) hits[at].classList.remove('on');
    at = (i + hits.length) % hits.length;
    hits[at].classList.add('on');
    hits[at].scrollIntoView({ block: 'center', behavior: 'smooth' });
    qn.textContent = (at + 1) + '/' + hits.length;
  }

  q.addEventListener('input', function () {
    clearTimeout(timer);
    var term = q.value;
    timer = setTimeout(function () { run(term); }, 150);
  });
  q.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') { e.preventDefault(); go(at + (e.shiftKey ? -1 : 1)); }
    if (e.key === 'Escape') { q.value = ''; clear(); q.blur(); }
  });
  document.getElementById('qnx').addEventListener('click', function () { go(at + 1); });
  document.getElementById('qp').addEventListener('click', function () { go(at - 1); });

  // "/" focuses the search, the way every reader expects it to.
  document.addEventListener('keydown', function (e) {
    if (e.key === '/' && document.activeElement !== q) { e.preventDefault(); q.focus(); }
  });

  // Arriving from a governance action's alignment links.
  {{if .Focus}}
  var target = document.getElementById({{.Focus}});
  if (target) target.scrollIntoView({ block: 'start' });
  {{end}}
})();
</script>
</body>
</html>`
