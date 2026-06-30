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

1. **Standalone core** , lift the governance workflow out of WordPress into a self-hostable service (containerized, configured via environment).
2. **Governance-action ingestion** , reliable read of on-chain governance actions.
3. **Constitutionality review** , pluggable LLM and rule-based checks against the Cardano Constitution.
4. **Deliberation & internal voting** , member discussion and internal vote capture.
5. **Rationale & on-chain submission** , author the committee rationale and package + submit the vote on-chain.
6. **Docs, tests, SBOM, and release hygiene** , maintainability and reliability for downstream committees.

This roadmap is maintained alongside the milestones defined under the Intersect OSC Tooling Sustainability Program.
