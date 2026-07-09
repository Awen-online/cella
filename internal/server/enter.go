package server

import "net/http"

// handleEnter renders the entry splash: a branded welcome to the body's private
// chamber, the member roster, a real wallet sign-in, and a demo fallback.
func (s *Server) handleEnter(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.member(r); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.etpl.Execute(w, demoBody); err != nil {
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
  a:focus-visible, button:focus-visible { outline:2px solid var(--goldb); outline-offset:3px; }
  footer { color:var(--muted); font-size:12.5px; margin-top:40px; text-align:center; font-family:'JetBrains Mono',ui-monospace,monospace; }
  @media (prefers-reduced-motion:no-preference){ h1.salve{ animation:fade .6s ease both; } @keyframes fade{ from{opacity:0; transform:translateY(6px);} to{opacity:1;} } }
</style>
</head>
<body>
  <svg class="badge" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" role="img" aria-label="Cella">
    <rect width="100" height="100" rx="22" fill="#0A0E27"></rect>
    <g transform="translate(18,16) scale(0.64)">
      <path d="M22 86 L22 42 A28 28 0 0 1 78 42 L78 86" fill="none" stroke="#FAF7EE" stroke-width="9"></path>
      <rect x="11" y="84" width="78" height="9" rx="1.5" fill="#FAF7EE"></rect>
      <circle cx="50" cy="62" r="6.5" fill="#F5D27A"></circle>
    </g>
  </svg>
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

    <div class="rule"></div>
    <div class="roster-label">The body — {{len .Members}} delegates</div>
    <div class="roster">
      {{range .Members}}
      <div class="m">
        <div class="name">{{.Name}}</div>
        <div class="role">{{.Role}}</div>
        <div class="addr">{{.Address}}</div>
        <form method="post" action="/auth/member">
          <input type="hidden" name="member" value="{{.Name}}">
          <button class="enter" type="submit">Enter as this member (demo)</button>
        </form>
      </div>
      {{end}}
    </div>
  </div>

  <footer>Cella · self-hostable Constitutional Committee governance · Apache-2.0</footer>

  <script>
  // Real CIP-30 connect-and-sign is wired in the next pass; for now the button
  // guides the presenter to the demo entry while wallet support lands.
  document.getElementById('btn-wallet').addEventListener('click', function () {
    var msg = document.getElementById('wallet-msg');
    var has = window.cardano && Object.keys(window.cardano).some(function (k) {
      return window.cardano[k] && typeof window.cardano[k].enable === 'function';
    });
    msg.textContent = has
      ? 'Wallet detected — sign-in is being wired up. For now, enter as a member below.'
      : 'No Cardano wallet detected in this browser. Enter as a member below to continue.';
  });
  </script>
</body>
</html>`
