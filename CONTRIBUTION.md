# Contribution Guidelines

## Development

### Code of Conduct

We are committed to providing a friendly, safe, and welcoming environment for all contributors. Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

### Getting Started

1. **Fork the repository** on GitHub.
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/tools.git
   cd kagent
   ```
3. **Add the upstream repository** as a remote:
   ```bash
   git remote add upstream https://github.com/kagent-dev/tools.git
   ```
4. **Create a new branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

### Development Environment Setup

See the [DEVELOPMENT.md](DEVELOPMENT.md) file for more information.

### Making Changes

#### Coding Standards

- **Go Code**:
  - Follow the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
  - Use the official MCP SDK patterns: `github.com/modelcontextprotocol/go-sdk`
  - Run `make lint` before submitting your changes
  - Ensure all tests pass with `make test`
  - Add tests for new functionality
  - Follow MCP specification for tool implementations

#### Commit Guidelines

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that do not affect the meaning of the code
- **refactor**: A code change that neither fixes a bug nor adds a feature
- **perf**: A code change that improves performance
- **test**: Adding missing tests or correcting existing tests
- **chore**: Changes to the build process or auxiliary tools

Example commit message:
```
feat(controller): add support for custom resource validation

This adds validation for the KagentApp custom resource to ensure
that the configuration is valid before applying it to the cluster.

Closes #123
```

### Pull Request Process

1. **Update your fork** with the latest changes from upstream:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Push your changes** to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

3. **Create a Pull Request** from your fork to the main repository.

4. **Fill out the PR template** with all required information.

5. **Address review comments** if requested by maintainers.

6. **Update your PR** if needed:
   ```bash
   git add .
   git commit -m "address review comments"
   git push origin feature/your-feature-name
   ```

7. Once approved, a maintainer will merge your PR.


### Documentation

- Update documentation for any changes to APIs, CLIs, or user-facing features
- Add examples for new features
- Update the README if necessary
- Add comments to your code explaining complex logic

### Releasing

Only project maintainers can create releases. The process is:

1. Update version numbers in relevant files
2. Create a release branch
3. Create a tag for the release
4. Build and publish artifacts
5. Create a GitHub release with release notes

### Community

- Join our [Discord server](https://discord.gg/Fu3k65f2k3) for discussions
- Participate in community calls (scheduled on our website)
- Help answer questions in GitHub issues
- Review pull requests from other contributors

## License

By contributing to this project, you agree that your contributions will be licensed under the project's license.

## Questions?

If you have any questions about contributing, please open an issue or reach out to the maintainers.
