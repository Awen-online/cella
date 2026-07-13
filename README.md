<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo/cella-lockup-dark.png" />
    <img src="assets/brand/logo/cella-lockup-light.png" alt="Cella" width="340" />
  </picture>
</p>

# Cella

> Open-source, self-hostable infrastructure for Cardano Constitutional Committee (CC) governance.

Cella gives Constitutional Committee members and consortia a single workflow for their on-chain duties: ingest governance actions, assess constitutionality with LLM assistance and other checks, deliberate with members, vote internally, author the committee's final rationale, and package and submit the vote and rationale on-chain.

It runs standalone (no WordPress required), so any Constitutional Committee or consortium, such as [Cardano Curia](https://cardanocuria.com) can self-host it.

## What it does

- **Ingest governance actions** , pull and present on-chain governance actions for review.
- **Constitutionality review** , connect to large language models and other tooling to assess whether a governance action aligns with the Cardano Constitution.
- **Deliberation** , structured discussion among committee / consortium members.
- **Internal voting** , members vote internally to settle the committee's position.
- **Rationale** , author the committee's final, citable rationale for its vote as a [CIP-136](https://github.com/cardano-foundation/CIPs/tree/master/CIP-0136) document, with the real on-chain anchor hash.
- **On-chain submission** , package the vote and rationale and submit them on-chain.

## The rationale is a real artifact

Cella's committee rationale is not a preview or a mock-up. Authoring one produces a genuine CIP-136 JSON-LD document, downloadable at `/rationale/{action}.jsonld`, and Cella shows you its **anchor hash** — the blake2b-256 of exactly those bytes, and precisely the value that goes on-chain with the vote:

```bash
cardano-cli hash anchor-data --file-text rationale-<action>.jsonld
# prints the same hash Cella displays
```

The document's `internalVote` block is filled from the chamber's own deliberation: each delegate voting Yes counts as *constitutional*, No as *unconstitutional*, and roster members who never took a position as *didNotVote* — which is how a multi-member committee shows the chain that its single vote came out of a real internal split. Delegates who recorded a position are named as the document's authors.

Cella will not hand you a document that CIP-136 would reject: an anchor hash over an incomplete rationale looks submittable when it isn't, so the download is refused until the rationale is complete.

## Who it's for

Cardano Constitutional Committee members, CC consortia (such as Cardano Curia), and any governance body that needs a transparent, repeatable path from a governance action to an on-chain committee vote.

## Status

Cella is being rebuilt from an existing WordPress deployment (live at [cardanocuria.com](https://cardanocuria.com)) into a **standalone, single-binary tool**: no WordPress, no database server, no runtime dependencies. The first slice (governance-action ingest + web view) is in this repo. See [MAINTAINERS.md](MAINTAINERS.md) for the roadmap.

## Quickstart

Cella is a single Go binary. Build it once, then run. It creates its own local SQLite database.

```bash
git clone https://github.com/Awen-online/cella
cd cella
go mod tidy         # fetch dependencies (first build only)
go build -o cella .

./cella ingest      # pull governance actions from Koios into ./cella.db
./cella serve       # then open http://localhost:8080
```

That's the whole install. No API key is required — Koios is a public, decentralized Cardano query layer.

### Try it without a wallet

Wallet sign-in is the only way into a real instance, so to look around locally without one, run in demo mode:

```bash
CELLA_DEMO=1 ./cella serve
```

This enables a roster picker on the entry splash that signs you in as any delegate you click, **with no proof of identity at all**. It exists to demonstrate the chamber. Never set it on an instance anyone else can reach: a stranger could vote in a delegate's name and author the committee's rationale, which is then anchored on-chain and cited permanently.

### Running it for real

```bash
export CELLA_SECRET=$(openssl rand -hex 32)   # signs session cookies
export CELLA_BODY=body/curia.json             # who you are — see body/curia.json
export CELLA_HOT_NFT_ADDR=addr1w...           # the committee's hot NFT script address

./cella ingest && ./cella serve
```

### Configuration

Everything is by environment; every value has a sensible default. See [`.env.example`](.env.example) for the full annotated list.

| Variable | What it does |
|---|---|
| `CELLA_DB` | SQLite database path (default `./cella.db`) |
| `CELLA_ADDR` | Listen address (default `:8080`) |
| `CELLA_SECRET` | **Signs session cookies.** Unset means a random key per start — secure, but every restart signs everyone out. Set it for any persistent deployment. |
| `CELLA_BODY` | Path to the body JSON — **who this instance belongs to**. A consortium or a single member; both are supported. Its logo sits as a file alongside it. See [`body/curia.json`](body/curia.json). Cella recognises a member by hashing the public key in their wallet signature and matching it against their registered address, so **a member with no address cannot sign in.** Unset, Cella uses the built-in Cardano Curia body. |
| `CELLA_HOT_NFT_ADDR` | The hot NFT script address. Its inline datum names the voting group, and therefore what quorum actually is. Cella reads this from the chain rather than trusting local config — without it, quorum is reported as unknown rather than guessed. |
| `CELLA_DEMO` | **Disables authentication.** Enables the roster picker. Local demos only. |
| `CELLA_NETWORK` | `mainnet` (default), `preprod` or `preview`. Picks the Koios endpoint **and** the explorer links, so a committee practising on a testnet is on that testnet everywhere. An unrecognised name is a startup error, never a silent mainnet. **SanchoNet is not supported** — Koios does not serve it; governance is live on Preprod. |
| `KOIOS_URL` | Override the network's Koios endpoint, for a private instance. |
| `KOIOS_TOKEN` | Optional Koios bearer token (higher rate limits). |
| `CELLA_LLM_URL` / `CELLA_LLM_MODEL` / `CELLA_LLM_KEY` | Bring-your-own model for `cella review`. |

## Tests

```bash
go test ./...            # the whole suite
go test ./... -cover     # with coverage
```

Two of these are worth knowing about, because they are what stop Cella from lying to a committee:

- **`internal/rationale`** hashes CIP-136's *own published example document* and asserts it reproduces the file hash the CIP publishes for it. If that fails, every anchor hash Cella produces is wrong.
- **`internal/cardano`** decodes a *real* credential-manager hot NFT datum — one whose blake2b-256 is the inline datum hash IntersectMBO publishes — and pins the quorum rule at `ceil(n/2)`, so a group of four needs two signatures rather than the intuitive three.

## Part of the Awen ecosystem

Cella is built and maintained by [Awen](https://awen.online), a studio building a federation of Web3, civic, and creative tools.

## Brand & assets

Brand guidelines and logo marks live in [`assets/brand/`](assets/brand/). See [`assets/brand/BRAND.md`](assets/brand/BRAND.md) for the full specification (color tokens, typography, components), and [`assets/brand/logo/`](assets/brand/logo/) for the marks: two directions (*aedicula* and *chamber*), each in gold-leaf, gold-solid, ink, and ivory-on-forum finishes.

## License

[Apache-2.0](LICENSE).

## Contributing & security

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).
