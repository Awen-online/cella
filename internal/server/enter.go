package server

import "net/http"

// enterView is the entry splash: the body, plus whether the roster picker is
// offered. The roster itself is always shown — it is who the body is — but the
// "enter as this member" buttons only appear in demo mode.
type enterView struct {
	Body
	Demo bool
}

// handleEnter renders the entry splash: a branded welcome to the body's private
// chamber, the member roster, and wallet sign-in.
func (s *Server) handleEnter(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.member(r); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.etpl.Execute(w, enterView{Body: s.body, Demo: s.demo}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// enterHTML is the entry splash (Cella-branded, dark chamber).
const enterHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Enter — {{.Name}} · Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#d7ddef; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --goldd:#7A5418; --blue:#1E5BFF; --blue-l:#6f93ff; --green:#4bbd88; }
  * { box-sizing:border-box; }
  body { margin:0; min-height:100vh; background:radial-gradient(120% 80% at 50% -10%, #131A40 0%, var(--forum) 60%); color:var(--body);
    font-family:'EB Garamond',Georgia,'Times New Roman',serif; display:flex; flex-direction:column; align-items:center; padding:48px 20px 60px; }
  .badge { width:74px; height:74px; }
  .bodymark { width:auto; height:96px; max-width:min(420px,80vw); }
  .eyebrow { font-family:'Cinzel','Trajan Pro',Georgia,serif; color:var(--gold); font-size:12px; letter-spacing:.34em; text-transform:uppercase; margin:22px 0 6px; }
  h1.salve { font-family:'Cinzel','Trajan Pro',Georgia,serif; color:var(--ivory); font-weight:800; letter-spacing:.14em; font-size:clamp(30px,6vw,46px); margin:0; text-align:center; }
  .lede { max-width:620px; text-align:center; font-size:18px; line-height:1.6; margin:16px auto 4px; }
  .lede b { color:var(--ivory); }
  .kind { color:var(--gold); font-family:'Cinzel',serif; font-size:11px; letter-spacing:.18em; text-transform:uppercase; text-align:center; margin-bottom:4px; }
  .wrap { width:100%; max-width:760px; }
  .rule { height:1px; background:linear-gradient(90deg,transparent,var(--goldd),var(--gold),var(--goldd),transparent); margin:30px 0 22px; }
  .roster-label { font-family:'Cinzel',serif; color:var(--gold); font-size:11px; letter-spacing:.16em; text-transform:uppercase; text-align:center; margin-bottom:14px; }
  .roster { display:grid; grid-template-columns:repeat(auto-fill,minmax(220px,1fr)); gap:12px; }
  .m { background:var(--veil); border:1px solid rgba(201,137,42,.22); border-radius:12px; padding:14px 15px; }
  .m .name { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:15px; letter-spacing:.03em; }
  .m .role { color:var(--muted); font-size:13.5px; margin:3px 0 8px; }
  .m .handle { display:inline-block; color:var(--blue-l); text-decoration:none; font-size:13px; margin-bottom:6px; }
  .m .handle:hover { color:var(--goldb); }
  .m .addr { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; color:var(--goldb); font-size:11px; word-break:break-all; opacity:.85; }
  .m form { margin-top:10px; }
  .m .enter { width:100%; background:transparent; border:1px solid rgba(201,137,42,.4); color:var(--gold); font-family:'Cinzel',serif;
    font-size:11px; letter-spacing:.1em; text-transform:uppercase; border-radius:999px; padding:7px 10px; cursor:pointer; }
  .m .enter:hover { background:rgba(245,210,122,.1); color:var(--goldb); }
  .connect { text-align:center; margin-top:30px; }
  .btn-wallet { font-family:'Cinzel',serif; font-size:14px; letter-spacing:.12em; text-transform:uppercase; font-weight:600; color:#fff;
    background:linear-gradient(180deg,var(--blue-l),var(--blue)); border:0; border-radius:12px; padding:15px 34px; cursor:pointer; box-shadow:0 8px 24px rgba(30,91,255,.35); }
  .btn-wallet:hover { filter:brightness(1.06); }
  .connect .hint { color:var(--muted); font-size:13px; margin-top:12px; }
  .connect .hint b { color:var(--body); }
  #wallet-msg { color:var(--goldb); font-size:13px; margin-top:10px; min-height:1.2em; }
  #wallet-modal { display:none; position:fixed; inset:0; background:rgba(8,10,26,.82); z-index:100; align-items:center; justify-content:center; padding:20px; }
  #wallet-modal .box { background:var(--veil); border:1px solid rgba(201,137,42,.3); border-radius:16px; padding:22px 22px 18px; max-width:360px; width:100%; }
  #wallet-modal h3 { font-family:'Cinzel',serif; color:var(--ivory); font-size:13px; letter-spacing:.12em; text-transform:uppercase; margin:0 0 14px; text-align:center; }
  #wallet-list { display:flex; flex-direction:column; gap:8px; }
  .wpick { display:flex; align-items:center; gap:12px; width:100%; text-align:left; background:var(--forum); border:1px solid rgba(201,137,42,.25); border-radius:10px; padding:12px 14px; cursor:pointer; color:var(--ivory); font-family:'EB Garamond',Georgia,serif; font-size:16px; }
  .wpick:hover { border-color:var(--goldb); }
  .wpick img { width:26px; height:26px; border-radius:6px; flex:0 0 auto; }
  .wpick span { text-transform:capitalize; }
  #wallet-modal .cancel { display:block; margin:14px auto 0; background:none; border:0; color:var(--muted); font-size:13px; cursor:pointer; }
  .demo-warn { margin-top:26px; padding:12px 16px; border:1px dashed rgba(217,105,95,.55); border-radius:10px; background:rgba(217,105,95,.08); color:#e8a49c; font-size:13.5px; line-height:1.55; text-align:center; }
  .demo-warn b { color:#f0b8b1; }
  .demo-warn code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:12px; color:var(--goldb); }
  a:focus-visible, button:focus-visible { outline:2px solid var(--goldb); outline-offset:3px; }
  .blinks { display:flex; gap:18px; justify-content:center; margin-top:34px; }
  .blinks a { color:var(--muted); text-decoration:none; font-size:13px; }
  .blinks a:hover { color:var(--goldb); }
  footer { color:var(--muted); font-size:12.5px; margin-top:18px; text-align:center; font-family:'JetBrains Mono',ui-monospace,monospace; }
  @media (prefers-reduced-motion:no-preference){ h1.salve{ animation:fade .6s ease both; } @keyframes fade{ from{opacity:0; transform:translateY(6px);} to{opacity:1;} } }
</style>
</head>
<body>
  {{if .Logo}}
  <img class="bodymark" src="{{.Logo}}" alt="{{.Name}}">
  {{else}}
  <svg class="badge" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" role="img" aria-label="Cella">
    <rect width="100" height="100" rx="22" fill="#0A0E27"></rect>
    <g transform="translate(18,16) scale(0.64)">
      <path d="M22 86 L22 42 A28 28 0 0 1 78 42 L78 86" fill="none" stroke="#FAF7EE" stroke-width="9"></path>
      <rect x="11" y="84" width="78" height="9" rx="1.5" fill="#FAF7EE"></rect>
      <circle cx="50" cy="62" r="6.5" fill="#F5D27A"></circle>
    </g>
  </svg>
  {{end}}
  <div class="eyebrow">Salve, member</div>
  <h1 class="salve">The Private Chamber</h1>
  <div class="wrap">
    <p class="lede">You are entering the private chamber of <b>{{.Name}}</b>.</p>
    <div class="kind">{{.Kind}}</div>
    <p class="lede" style="font-size:16px;color:var(--body);opacity:.9;">{{.Blurb}}</p>

    <div class="connect">
      <button class="btn-wallet" id="btn-wallet" type="button">Connect wallet to enter</button>
      <div id="wallet-msg"></div>
      <div class="hint">Sign a challenge with your Cardano wallet to authenticate. <b>No funds move.</b></div>
    </div>

    {{if .Demo}}
    <div class="demo-warn">
      <b>Demo mode.</b> Anyone may enter as any delegate below, without proving who they are. Never run a reachable instance this way — unset <code>CELLA_DEMO</code> and sign in with a wallet.
    </div>
    {{end}}

    <div class="rule"></div>
    <div class="roster-label">{{if .Solo}}The member{{else}}The body — {{len .Members}} delegates{{end}}</div>
    <div class="roster">
      {{range .Members}}
      <div class="m">
        <div class="name">{{.Name}}</div>
        <div class="role">{{.Role}}</div>
        {{if .Handle}}<a class="handle" href="{{.Link}}" target="_blank" rel="noopener">{{.Handle}}</a>{{end}}
        {{if .Address}}<div class="addr">{{.Address}}</div>{{end}}
        {{if $.Demo}}
        <form method="post" action="/auth/member">
          <input type="hidden" name="member" value="{{.Name}}">
          <button class="enter" type="submit">Enter as this member (demo)</button>
        </form>
        {{end}}
      </div>
      {{end}}
    </div>
  </div>

  <div id="wallet-modal" role="dialog" aria-modal="true" aria-label="Choose a wallet">
    <div class="box">
      <h3>Choose a wallet</h3>
      <div id="wallet-list"></div>
      <button type="button" class="cancel" id="wallet-cancel">Cancel</button>
    </div>
  </div>

  <div class="blinks">
    {{if .Website}}<a href="{{.Website}}" target="_blank" rel="noopener">{{.Website}} ↗</a>{{end}}
    {{if .X}}<a href="{{.X}}" target="_blank" rel="noopener">X ↗</a>{{end}}
  </div>
  <footer>Powered by Cella · self-hostable Constitutional Committee governance · Apache-2.0</footer>

  <script>
  // Real CIP-30 connect-and-sign with a wallet picker: choose a wallet, it signs
  // a server-issued challenge (CIP-8), the server verifies the Ed25519 signature.
  (function () {
    var btn = document.getElementById('btn-wallet');
    var msg = document.getElementById('wallet-msg');
    var modal = document.getElementById('wallet-modal');
    var list = document.getElementById('wallet-list');
    var cancel = document.getElementById('wallet-cancel');

    // Only point people at the roster picker when it is actually there.
    var DEMO = {{.Demo}};
    var fallback = DEMO ? ' Enter as a member below.' : ' A wallet is the only way in.';

    function errText(e) {
      if (!e) return 'unknown';
      if (typeof e === 'string') return e;
      if (e.info) return e.info;
      if (e.message) return e.message;
      try { return JSON.stringify(e); } catch (x) { return String(e); }
    }
    function wallets() {
      return Object.keys(window.cardano || {}).filter(function (k) {
        var w = window.cardano[k];
        return w && typeof w.enable === 'function' && typeof w.icon !== 'undefined';
      });
    }

    async function signIn(key) {
      modal.style.display = 'none';
      var w = window.cardano[key];
      try {
        msg.textContent = 'Connecting ' + (w.name || key) + '…';
        var api = await w.enable();
        var rew = await api.getRewardAddresses();
        var addr = (rew && rew[0]) || (await api.getUsedAddresses())[0];
        if (!addr) { msg.textContent = 'Could not read an address from ' + (w.name || key) + '.'; return; }
        var ch = await (await fetch('/auth/challenge')).json();
        msg.textContent = 'Approve the signature in ' + (w.name || key) + '…';
        var signed = await api.signData(addr, ch.challenge);
        var res = await fetch('/auth/verify', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ address: addr, signature: signed.signature, key: signed.key })
        });
        if (res.ok) { location.href = '/'; return; }
        var je = await res.json();
        msg.textContent = 'Sign-in failed (' + (je.error || 'unknown') + ').' + fallback;
      } catch (err) {
        msg.textContent = 'Sign-in cancelled or failed (' + errText(err) + ').' + fallback;
      }
    }

    btn.addEventListener('click', function () {
      var ws = wallets();
      if (!ws.length) { msg.textContent = 'No Cardano wallet found in this browser. Install Eternl or Lace.' + fallback; return; }
      if (ws.length === 1) { signIn(ws[0]); return; }
      list.innerHTML = '';
      ws.forEach(function (k) {
        var w = window.cardano[k];
        var b = document.createElement('button'); b.type = 'button'; b.className = 'wpick';
        b.innerHTML = (w.icon ? '<img src="' + w.icon + '" alt="">' : '') + '<span>' + (w.name || k) + '</span>';
        b.addEventListener('click', function () { signIn(k); });
        list.appendChild(b);
      });
      modal.style.display = 'flex';
    });
    cancel.addEventListener('click', function () { modal.style.display = 'none'; });
    modal.addEventListener('click', function (e) { if (e.target === modal) modal.style.display = 'none'; });
  })();
  </script>
</body>
</html>`
