# Changelog

All notable changes to Cella are documented here. The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- **Standalone Go core.** Cella is now a single, self-contained binary — no WordPress, no database server, no CGO. Local persistence is SQLite via a pure-Go driver.
- **Governance-action ingest** (`cella ingest`) — pull on-chain governance actions and Constitutional Committee votes from Koios into the local database, with CIP-108 anchor metadata parsed for titles and abstracts.
- **AI-assisted constitutionality review** (`cella review`) — assess stored actions against the Cardano Constitution using any OpenAI-compatible model (bring your own: OpenAI, OpenRouter, Groq, vLLM, LM Studio, local Ollama). The review is grounded in the actual Constitution text, not the model's recollection of it.
- **Embedded Constitution** — the versioned Constitution (v0 interim, v1, v2.4 current) ships inside the binary and is served as a browsable view with revision switching.
- **Web UI** (`cella serve`) — governance actions index and per-action detail pages: metadata, abstract, the full Constitutional Committee vote roster with rationales, and the AI review.
- **Entry chamber** — an entry splash and session gate fronting the private chamber, with the delegate body roster.
- **Wallet sign-in** — real CIP-30 wallet authentication: the browser signs a server-issued challenge (CIP-8 / COSE_Sign1) and the server verifies the Ed25519 signature. No key ever leaves the wallet.
- **Chamber deliberation** — per-member internal vote casting with rationales on a shared instance, so co-delegates see each other's positions before the committee casts its single on-chain vote.
- **Constitutional Committee roster** — all 7 authorized seats, with seats that have not yet voted shown as awaiting.
- **On-chain submission flow** — walks the credential-manager / hot-NFT multisig path (anchor, compose, build, co-sign, submit).
- Initial public repository: README, governance docs (CONTRIBUTING, CODE_OF_CONDUCT, GOVERNANCE, MAINTAINERS, SUPPORT), SECURITY policy, Apache-2.0 LICENSE, CI workflow, and Dependabot configuration.

- **Deadline countdowns.** Governance actions expire, and an action that expires unvoted is an abstention the committee never chose — so the clock is now the first column on the dashboard, with quorum progress beside it. Urgency is colour-coded (under two days is critical, under five is soon) and the countdown ticks live so a page left open does not go quietly stale.

- **Committee rationale (CIP-136)** — author the committee's citable rationale for its vote and emit it as a real CIP-136 JSON-LD document, downloadable at `/rationale/{action}.jsonld`. Cella computes and displays the document's **anchor hash** (blake2b-256 of the exact served bytes) — the same value `cardano-cli hash anchor-data --file-text` prints, and the one submitted on-chain with the vote. The `internalVote` block is derived from the chamber's own deliberation, and the delegates who recorded a position are named as authors. A rationale that CIP-136 would reject cannot be downloaded, because an anchor hash over an incomplete document looks submittable when it is not.

### Fixed
- **Fixed: every action claimed to have expired in 1970.** The chain states an action's expiration as an epoch *number*, but Cella formatted it as a Unix timestamp — so `epoch 648` rendered as `1970-01-01`, and the dashboard showed no expiry at all. Cella now captures the network's genesis parameters at ingest (via Koios `/genesis`) and derives a real wall-clock deadline: the end of the expiration epoch, since an action stays votable through it. The arithmetic is pinned in tests against Koios's own slot numbers, and where the genesis parameters are unavailable Cella shows the raw epoch and no countdown rather than inventing a date.

### Changed
- **The chamber shows only real positions.** Member stances were previously generated deterministically from the proposal id to illustrate the flow. They are gone: the chamber now shows exactly what delegates have recorded, and a delegate who has not voted is shown as awaiting rather than being given an invented opinion. The committee's decision, its internal split, and the submission flow all derive from recorded votes alone, and a tie abstains rather than being resolved into a mandate.
- On-chain submission is now gated on having an anchorable rationale — a committee votes with its reasoning attached — and displays the real anchor hash rather than a placeholder filename.
- CI now takes its Go version from `go.mod` rather than a separately pinned version, so the two cannot drift apart.

### Security
- **Roster sign-in is now off by default**, behind `CELLA_DEMO`. The entry splash's member picker signs a visitor in as whichever delegate they click, with no proof of identity — and it was reachable on every deployment, so anyone who could load the page could vote as any delegate and author the committee's rationale. It is now refused at the endpoint (not merely hidden in the page, which would leave a direct post working), and both the splash and the server log say plainly what demo mode gives away.
- **Session cookies are signed** (HMAC-SHA256, keyed by `CELLA_SECRET`). The cookie previously carried a delegate's identity in the clear, so anyone could set it to any name on the roster and record votes as that delegate. An unverifiable cookie is now treated as no session at all.
- **Vote and rationale posts require a CSRF token** bound to the session identity, so a token minted for one delegate cannot authorize a post as another.

### Notes
- On-chain submission does not broadcast a transaction: that requires the committee's cold keys. The rationale document and its anchor hash, however, are real — the same bytes and the same hash a committee would anchor.
