<p align="center">
  <img src="assets/brand/logo/cella-aedicula-gold-leaf.svg" alt="Cella" width="120" />
</p>

# Cella

> Open-source, self-hostable infrastructure for Cardano Constitutional Committee (CC) governance.

Cella gives Constitutional Committee members and consortia a single workflow for their on-chain duties: ingest governance actions, assess constitutionality with LLM assistance and other checks, deliberate with members, vote internally, author the committee's final rationale, and package and submit the vote and rationale on-chain.

It is the reusable core of [Cardano Curia](https://cardanocuria.com), extracted to run standalone (no WordPress required) so any committee or consortium can self-host it.

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

Cella is being extracted from the Curia plugin into standalone, self-hostable infrastructure. See [MAINTAINERS.md](MAINTAINERS.md) for the roadmap.

## Self-hosting

Cella is designed to run on any server. Container and quickstart instructions land with the standalone migration (see the roadmap). Configuration is via environment variables; see `.env.example`.

## Part of the Awen ecosystem

Cella is built and maintained by [Awen](https://awen.online), the studio behind Cardano Curia and a federation of Web3, civic, and creative tools.

## Brand & assets

Brand guidelines and logo marks live in [`assets/brand/`](assets/brand/). See [`assets/brand/BRAND.md`](assets/brand/BRAND.md) for the full specification (color tokens, typography, components), and [`assets/brand/logo/`](assets/brand/logo/) for the marks — two directions (*aedicula* and *chamber*), each in gold-leaf, gold-solid, ink, and ivory-on-forum finishes.

## License

[Apache-2.0](LICENSE).

## Contributing & security

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).
