<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo/cella-lockup-dark.png" />
    <img src="assets/brand/logo/cella-lockup-light.png" alt="Cella" width="340" />
  </picture>
</p>

# Cella

> Open-source, self-hostable infrastructure for Cardano Constitutional Committee (CC) governance.

Cella gives Constitutional Committee members and consortia a single workflow for their on-chain duties: ingest governance actions, assess constitutionality with LLM assistance and other checks, deliberate with members, vote internally, author the committee's final rationale, and package and submit the vote and rationale on-chain.

It runs standalone (no WordPress required), so any Constitutional Committee or consortium — such as [Cardano Curia](https://cardanocuria.com) — can self-host it.

## What it does

- **Ingest governance actions** , pull and present on-chain governance actions for review.
- **Constitutionality review** , connect to large language models and other tooling to assess whether a governance action aligns with the Cardano Constitution.
- **Deliberation** , structured discussion among committee / consortium members.
- **Internal voting** , members vote internally to settle the committee's position.
- **Rationale** , author the committee's final, citable rationale for its vote.
- **On-chain submission** , package the vote and rationale and submit them on-chain.

## Who it's for

Cardano Constitutional Committee members, CC consortia (such as Cardano Curia), and any governance body that needs a transparent, repeatable path from a governance action to an on-chain committee vote.

## Status

Cella is being rebuilt from an existing WordPress deployment (live at [cardanocuria.com](https://cardanocuria.com)) into a **standalone, single-binary tool** — no WordPress, no database server, no runtime dependencies. The first slice (governance-action ingest + web view) is in this repo. See [MAINTAINERS.md](MAINTAINERS.md) for the roadmap.

## Quickstart

Cella is a single Go binary. Build it once, then run — it creates its own local SQLite database.

```bash
git clone https://github.com/Awen-online/cella
cd cella
go mod tidy         # fetch dependencies (first build only)
go build -o cella .

./cella ingest      # pull governance actions from Koios into ./cella.db
./cella serve       # then open http://localhost:8080
```

That's the whole install. Configuration is optional and by environment (see [`.env.example`](.env.example)): `CELLA_DB`, `CELLA_ADDR`, `KOIOS_URL`, `KOIOS_TOKEN`. Point `KOIOS_URL` at a Preprod/Preview instance to work against testnets.

No API key is required — Koios is a public, decentralized Cardano query layer.

## Part of the Awen ecosystem

Cella is built and maintained by [Awen](https://awen.online), a studio building a federation of Web3, civic, and creative tools.

## Brand & assets

Brand guidelines and logo marks live in [`assets/brand/`](assets/brand/). See [`assets/brand/BRAND.md`](assets/brand/BRAND.md) for the full specification (color tokens, typography, components), and [`assets/brand/logo/`](assets/brand/logo/) for the marks — two directions (*aedicula* and *chamber*), each in gold-leaf, gold-solid, ink, and ivory-on-forum finishes.

## License

[Apache-2.0](LICENSE).

## Contributing & security

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).
