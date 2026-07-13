# Maintainers

Cella is built and maintained by **Awen LLC** , a digital studio , on behalf of the **Cardano Curia** Constitutional Committee consortium. Awen is the maintaining organization and the point of contact for sustainability funding.

## Maintaining organization

- **Org:** Awen LLC ([awen.online](https://awen.online))
- **Project home:** https://github.com/Awen-online/cella
- **Contact:** info@awen.online

## Maintainers & roles

| Maintainer | Role | Contact |
|---|---|---|
| Ian "Cullah" McCullough | Lead maintainer / engineering | cullah@awen.online |
| _TBD_ | Maintainer / reviewer | _add as the team grows_ |

Maintainers are responsible for reviewing and merging contributions, triaging issues and security reports, cutting releases, and stewarding the roadmap.

## Decision making

Changes are proposed via issues and pull requests. Maintainers review for scope, quality, security, and alignment with the project's sustainability focus. Significant or breaking changes are discussed in an issue before implementation.

## Roadmap

Cella is being extracted from the private Curia plugin into standalone, self-hostable infrastructure. Near-term direction:

1. **Standalone core** — ✅ done. A single Go binary, configured by environment. No WordPress, no database server, no CGO.
2. **Governance-action ingestion** — ✅ done. Actions and Constitutional Committee votes read from Koios, with CIP-108 anchor metadata and real expiry deadlines.
3. **Constitutionality review** — ✅ done. Pluggable LLM assessment grounded in the embedded Constitution text. Rule-based checks remain open.
4. **Deliberation & internal voting** — ✅ done. Delegates sign their positions with their Cardano wallet, so the record is attributable rather than merely asserted.
5. **Rationale & on-chain submission** — *in progress.* The rationale is done and real: Cella emits a genuine CIP-136 document and its true on-chain anchor hash, and reads the committee's quorum from the hot NFT datum rather than guessing it. Submission is not: Cella does not yet build, collect witnesses for, or broadcast the vote transaction. See below.
6. **Docs, tests, SBOM, and release hygiene** — ongoing. Test suite and CI are in place; SBOM is generated per build.

### What "submission" still needs

The remaining gap between Cella and an on-chain vote, in the order it should be closed:

- **Witness collection.** Cella publishes the transaction body, each delegate signs it offline with their voting key and uploads the witness, Cella tracks quorum and assembles. Cella never holds a key.
- **Transaction building.** Casting a committee vote spends a Plutus script UTxO, which requires execution-unit estimation. Go has no mature Conway-era Plutus transaction builder, so this means driving `cardano-cli` and `orchestrator-cli` — and accepting that live submission cannot keep the single-binary, no-runtime-dependencies promise that holds for everything up to the transaction.
- **IPFS pinning.** Cella produces the rationale's anchor *hash* but not its CID, so `--metadata-url` is still a manual step.

This roadmap is maintained alongside the milestones defined under the Intersect OSC Tooling Sustainability Program.
