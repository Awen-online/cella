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

### Changed
- CI now takes its Go version from `go.mod` rather than a separately pinned version, so the two cannot drift apart.

### Notes
- On-chain submission is currently a demonstration of the flow: it does not broadcast a transaction. Live submission requires the committee's cold keys.
