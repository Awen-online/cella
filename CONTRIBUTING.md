# Contributing to Cella

Thanks for your interest in improving Cella , open-source infrastructure for Cardano Constitutional Committee governance. Contributions of all kinds are welcome: bug reports, fixes, features, docs, and tests.

## Ways to contribute

- **Report a bug** , open an issue with steps to reproduce, expected vs. actual behavior, and your environment.
- **Propose a change** , open an issue describing the problem and your proposed approach before large PRs, so we can align early.
- **Submit a pull request** , see the workflow below.

## Scope

Cella sustains a focused workflow: governance-action ingestion → constitutionality review → deliberation → internal voting → rationale → on-chain submission. Contributions that strengthen maintainability, developer experience, interoperability, and reliability are prioritized. Net-new product directions are best discussed in an issue first.

## Development workflow

1. Fork the repo and create a branch from `main`: `git checkout -b feat/short-description`.
2. Make your change with clear, focused commits.
3. Add or update tests where it makes sense, and run the test suite locally.
4. Run linters/formatters so CI passes.
5. Open a pull request against `main` with a clear description of what and why.

## Commit & PR guidelines

- Keep PRs small and single-purpose where possible.
- Write descriptive commit messages (imperative mood, e.g. "Add rationale export").
- Reference related issues (e.g. `Fixes #123`).
- CI must pass before review.

## Code of conduct

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Security

Please do not file security issues publicly , see [SECURITY.md](SECURITY.md) for responsible disclosure.

## License

By contributing, you agree that your contributions are licensed under the project's [Apache-2.0](LICENSE) license.
