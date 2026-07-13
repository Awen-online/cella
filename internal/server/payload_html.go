package server

// The payload panel shows what a governance action actually *does*: who gets
// paid, which parameters change, which protocol version a hard fork targets.
//
// Until now Cella showed a delegate the title and the abstract — the pitch. A
// committee assessing constitutionality has to read the binding part. An
// abstract can say "a modest grant"; the payload says 120,000,000 ada to a
// script address, and only one of those is what the chain will execute.

// payloadHTML is included in the action detail template.
const payloadHTML = `
{{define "payload"}}
<div class="card">
  <h2>On-chain payload &mdash; what this action does</h2>

  {{if not .HasPayload}}
    <div class="muted">No payload recorded. Re-run <code>cella ingest</code> to fetch it.</div>

  {{else if not .Payload.Recognised}}
    <div class="pl-note">Cella does not recognise this action type (<code>{{.Payload.Kind}}</code>), so it cannot lay the payload out. The raw on-chain contents are below — read them before voting.</div>
    <pre class="pl-raw">{{printf "%s" .Payload.Unknown}}</pre>

  {{else if .Payload.Withdrawals}}
    {{with .Payload.Withdrawals}}
    <div class="pl-head"><b class="pl-total">&#8371;{{ada .Total}}</b> requested from the treasury{{if gt (len .Recipients) 1}}, across {{len .Recipients}} recipients{{end}}</div>
    <div class="pl-rows">
      {{range .Recipients}}
      <div class="pl-row">
        <div class="pl-amt">&#8371;{{ada .Lovelace}}</div>
        <div class="pl-barwrap"><div class="pl-bar" style="width:{{pct .Lovelace $.Payload.Withdrawals.Total}}%"></div></div>
        <div class="pl-pct">{{printf "%.1f" (pctf .Lovelace $.Payload.Withdrawals.Total)}}%</div>
        <div class="pl-cred">
          <span class="pl-net">{{.Network}}</span>
          <code>{{.Credential}}</code>{{if .IsScript}} <span class="pl-script">script</span>{{end}}
        </div>
      </div>
      {{end}}
    </div>
    {{if .ScriptHash}}<div class="pl-foot">Guardrails script <code>{{.ScriptHash}}</code></div>{{end}}
    {{end}}

  {{else if .Payload.Parameters}}
    {{with .Payload.Parameters}}
    <div class="pl-head"><b>{{len .Changes}}</b> protocol parameter{{if ne (len .Changes) 1}}s{{end}} would change</div>
    <table class="pl-params">
      <thead><tr><th>Parameter</th><th>Proposed value</th></tr></thead>
      <tbody>
        {{range .Changes}}<tr><td><code>{{.Name}}</code></td><td class="pl-val">{{.Proposed}}</td></tr>{{end}}
      </tbody>
    </table>
    <div class="pl-note">Cella shows the proposed values, not the current ones. Compare against the protocol parameters in force before voting — a value that looks reasonable in isolation may not be a reasonable <em>change</em>.</div>
    {{if .ScriptHash}}<div class="pl-foot">Guardrails script <code>{{.ScriptHash}}</code></div>{{end}}
    {{end}}

  {{else if .Payload.HardFork}}
    <div class="pl-head">Initiate a hard fork to protocol version <b class="pl-total">{{.Payload.HardFork.Version}}</b></div>
    <div class="pl-note">A hard fork changes the ledger rules themselves and must clear the highest thresholds. Stake pool operators must upgrade in coordination; verify node compatibility before voting.</div>

  {{else if .Payload.Committee}}
    {{with .Payload.Committee}}
    {{if .Quorum}}<div class="pl-head">Committee quorum would be <b class="pl-total">{{.Quorum}}</b></div>{{end}}
    {{if .Added}}
    <div class="pl-sub">Seats added</div>
    <div class="pl-rows">
      {{range .Added}}<div class="pl-seat"><code>{{.Credential}}</code> <span class="pl-net">term ends epoch {{.ExpiresAt}}</span></div>{{end}}
    </div>
    {{end}}
    {{if .Removed}}
    <div class="pl-sub">Seats removed</div>
    <div class="pl-rows">
      {{range .Removed}}<div class="pl-seat pl-removed"><code>{{.}}</code></div>{{end}}
    </div>
    {{end}}
    {{if and (not .Added) (not .Removed)}}<div class="muted">No seats added or removed; this action changes the quorum only.</div>{{end}}
    {{end}}

  {{else if .Payload.Constitution}}
    {{with .Payload.Constitution}}
    <div class="pl-head">This action would <b class="pl-total">replace the Constitution</b></div>
    <div class="pl-kv"><span>Anchor</span> <code>{{.AnchorURL}}</code></div>
    <div class="pl-kv"><span>Document hash</span> <code>{{.DataHash}}</code></div>
    {{if .ScriptHash}}<div class="pl-kv"><span>Guardrails script</span> <code>{{.ScriptHash}}</code></div>{{end}}
    <div class="pl-note">Read the proposed text at the anchor and compare it against the <a href="/constitution">Constitution currently in force</a>. Cella does not yet diff them for you.</div>
    {{end}}

  {{else if eq (printf "%s" .Payload.Kind) "NoConfidence"}}
    <div class="pl-head">A motion of <b class="pl-total">no confidence</b> in the Constitutional Committee</div>
    <div class="pl-note">If ratified, the committee is dissolved and every action requiring a committee vote stalls until a new one is seated. The payload carries no parameters — the meaning is entirely in the fact of it.</div>

  {{else}}
    <div class="pl-head">An <b class="pl-total">information action</b></div>
    <div class="pl-note">Nothing transacts and nothing changes on-chain. It records the collective position of the voting bodies and nothing more.</div>
  {{end}}
</div>
{{end}}
`

// votingContextHTML shows how the rest of the ecosystem is voting.
const votingContextHTML = `
{{define "votingcontext"}}
{{if .HasSummary}}
{{with .Summary}}
<div class="card">
  <h2>How the rest of the chain is voting</h2>
  <div class="vc-note">The committee decides constitutionality on its own reading, not by following the vote. But it should know what it is agreeing &mdash; or disagreeing &mdash; with.</div>

  <div class="vc-role">
    <div class="vc-name">Delegate representatives <span class="vc-n">{{.DRepVotes}} voted</span></div>
    {{if .DRepVotes}}
    <div class="vc-bar">
      <div class="vc-seg y" style="width:{{printf "%.2f" .DRepYesPct}}%" title="Yes"></div>
      <div class="vc-seg n" style="width:{{printf "%.2f" .DRepNoPct}}%" title="No"></div>
    </div>
    <div class="vc-legend">
      <span class="y">{{printf "%.1f" .DRepYesPct}}% Yes</span> &middot;
      <span class="n">{{printf "%.1f" .DRepNoPct}}% No</span> &middot;
      <span class="a">{{.DRepAbstain}} abstained</span>
      <span class="vc-cast">({{.DRepYesVotes}} yes / {{.DRepNoVotes}} no by count)</span>
    </div>
    {{else}}<div class="muted">No delegate representative has voted yet.</div>{{end}}
  </div>

  <div class="vc-role">
    <div class="vc-name">Stake pool operators <span class="vc-n">{{.PoolVotes}} voted</span></div>
    {{if .PoolVotes}}
    <div class="vc-bar">
      <div class="vc-seg y" style="width:{{printf "%.2f" .PoolYesPct}}%" title="Yes"></div>
      <div class="vc-seg n" style="width:{{printf "%.2f" .PoolNoPct}}%" title="No"></div>
    </div>
    <div class="vc-legend">
      <span class="y">{{printf "%.1f" .PoolYesPct}}% Yes</span> &middot;
      <span class="n">{{printf "%.1f" .PoolNoPct}}% No</span> &middot;
      <span class="a">{{.PoolAbstain}} abstained</span>
      <span class="vc-cast">({{.PoolYesVotes}} yes / {{.PoolNoVotes}} no by count)</span>
    </div>
    {{else}}<div class="muted">No stake pool operator has voted yet. Not every action type takes an SPO vote.</div>{{end}}
  </div>

  <div class="vc-foot">Percentages are of the voting power <em>recorded on this action</em>, not of total active stake.</div>
</div>
{{end}}
{{end}}
{{end}}
`
