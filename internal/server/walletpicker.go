package server

import "strings"

// A delegate signs twice: once to enter the chamber, and again on every position
// they record. Those must be the same wallet — the vote signature is checked
// against the key hash on the roster, so signing a vote with a different wallet
// than you signed in with fails, and fails confusingly.
//
// The entry splash had a picker. The ballot did not: it simply took the first
// wallet the browser happened to expose, which on a machine with several
// installed is whichever one loaded first, and not necessarily the one the
// delegate uses. So a delegate who entered with Eternl would be asked to sign
// their vote by Brave.
//
// One picker now serves both, and it remembers.

// walletPickerCSS styles the chooser. Injected into a page's <style> block.
const walletPickerCSS = `
  #cw-modal { display:none; position:fixed; inset:0; background:rgba(8,10,26,.82); z-index:1000; align-items:center; justify-content:center; padding:20px; }
  #cw-modal.open { display:flex; }
  #cw-modal .cw-box { background:var(--veil,#131A40); border:1px solid rgba(201,137,42,.3); border-radius:16px; padding:22px 22px 18px; max-width:380px; width:100%; }
  #cw-modal h3 { font-family:'Cinzel',serif; color:var(--ivory,#FAF7EE); font-size:13px; letter-spacing:.12em; text-transform:uppercase; margin:0 0 6px; text-align:center; }
  #cw-modal .cw-sub { color:var(--muted,#8b93b8); font-size:13px; text-align:center; margin:0 0 14px; }
  #cw-list { display:flex; flex-direction:column; gap:8px; }
  .cw-pick { display:flex; align-items:center; gap:12px; width:100%; text-align:left; background:var(--forum,#0A0E27); border:1px solid rgba(201,137,42,.25); border-radius:10px; padding:12px 14px; cursor:pointer; color:var(--ivory,#FAF7EE); font-family:'EB Garamond',Georgia,serif; font-size:16px; }
  .cw-pick:hover { border-color:var(--goldb,#F5D27A); }
  .cw-pick img { width:26px; height:26px; border-radius:6px; flex:0 0 auto; }
  .cw-pick .cw-name { text-transform:capitalize; }
  .cw-pick .cw-last { margin-left:auto; font-family:'Cinzel',serif; font-size:9px; letter-spacing:.08em; text-transform:uppercase; color:var(--green,#4bbd88); border:1px solid rgba(75,189,136,.5); border-radius:999px; padding:2px 8px; }
  #cw-modal .cw-cancel { display:block; margin:14px auto 0; background:none; border:0; color:var(--muted,#8b93b8); font-size:13px; cursor:pointer; }
  #cw-modal .cw-none { color:var(--muted,#8b93b8); font-size:14px; text-align:center; line-height:1.55; }
`

// walletPickerJS is the chooser itself. It exposes window.cellaWallet.
const walletPickerJS = `
<div id="cw-modal" role="dialog" aria-modal="true" aria-label="Choose a wallet">
  <div class="cw-box">
    <h3>Choose a wallet</h3>
    <div class="cw-sub" id="cw-sub">Sign with the wallet registered to you.</div>
    <div id="cw-list"></div>
    <button type="button" class="cw-cancel" id="cw-cancel">Cancel</button>
  </div>
</div>
<script>
window.cellaWallet = (function () {
  var KEY = 'cella.wallet';   // the wallet this delegate last signed with

  function list() {
    return Object.keys(window.cardano || {}).filter(function (k) {
      var w = window.cardano[k];
      return w && typeof w.enable === 'function' && typeof w.icon !== 'undefined';
    });
  }
  function nameOf(k) { var w = window.cardano[k]; return (w && w.name) || k; }
  function remembered() {
    try { return localStorage.getItem(KEY); } catch (e) { return null; }
  }
  function remember(k) {
    try { localStorage.setItem(KEY, k); } catch (e) { /* private mode: fine */ }
  }

  // pick resolves with a wallet key, or rejects if the delegate cancels.
  //
  // With one wallet installed there is no choice to make, so it does not ask.
  // With several it always asks — a delegate about to put their name to a
  // position should see which key is about to sign it — but it puts the wallet
  // they used last at the top, marked, so the common case is one click.
  function pick(subtitle) {
    return new Promise(function (resolve, reject) {
      var keys = list();
      if (!keys.length) {
        reject(new Error('No Cardano wallet found in this browser. Install Eternl or Lace.'));
        return;
      }
      if (keys.length === 1) { resolve(keys[0]); return; }

      var last = remembered();
      if (last && keys.indexOf(last) > 0) {           // float the remembered one
        keys = [last].concat(keys.filter(function (k) { return k !== last; }));
      }

      var modal = document.getElementById('cw-modal');
      var listEl = document.getElementById('cw-list');
      var sub = document.getElementById('cw-sub');
      if (subtitle) sub.textContent = subtitle;

      function close() {
        modal.classList.remove('open');
        listEl.innerHTML = '';
        document.removeEventListener('keydown', onKey);
      }
      function onKey(e) { if (e.key === 'Escape') { close(); reject(new Error('cancelled')); } }

      listEl.innerHTML = '';
      keys.forEach(function (k) {
        var w = window.cardano[k];
        var b = document.createElement('button');
        b.type = 'button';
        b.className = 'cw-pick';
        b.innerHTML =
          (w.icon ? '<img src="' + w.icon + '" alt="">' : '') +
          '<span class="cw-name">' + nameOf(k) + '</span>' +
          (k === last ? '<span class="cw-last">last used</span>' : '');
        b.addEventListener('click', function () {
          close();
          remember(k);
          resolve(k);
        });
        listEl.appendChild(b);
      });

      document.getElementById('cw-cancel').onclick = function () { close(); reject(new Error('cancelled')); };
      modal.onclick = function (e) { if (e.target === modal) { close(); reject(new Error('cancelled')); } };
      document.addEventListener('keydown', onKey);
      modal.classList.add('open');
    });
  }

  return { list: list, nameOf: nameOf, remembered: remembered, remember: remember, pick: pick };
})();
</script>
`

// withWalletPicker injects the chooser's styles into a page's <style> block and
// its markup and script just before </body>.
func withWalletPicker(h string) string {
	h = strings.Replace(h, "<style>", "<style>"+walletPickerCSS, 1)
	return strings.Replace(h, "</body>", walletPickerJS+"</body>", 1)
}
