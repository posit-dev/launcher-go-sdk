# Contributing to Launcher Go SDK

Thank you for your interest in contributing to the Launcher Go SDK!

## External contributions

We appreciate your interest in improving the Launcher Go SDK! While we welcome external contributions, please note that we evaluate pull requests on a case-by-case basis and may not be able to accept all submissions due to maintenance, compatibility, and strategic considerations.

Before investing significant time in a pull request, we strongly encourage you to:

1. **Open an issue first** to discuss your proposed changes
2. **Wait for feedback** from maintainers on whether the change aligns with project goals
3. **Keep contributions focused** on a specific bug fix or feature

This approach helps ensure your time is well spent and increases the likelihood that your contribution can be accepted. We value your understanding and look forward to collaborating where it makes sense for the project!

## Table of contents

1. [Code of Conduct](#code-of-conduct)
2. [Reporting Issues](#reporting-issues)
3. [Development Setup](#development-setup)
4. [Making Changes](#making-changes)
5. [Testing](#testing)
6. [Documentation](#documentation)
7. [Pull Request Process](#pull-request-process)
8. [Code Style](#code-style)
9. [Commit Messages](#commit-messages)

## Code of conduct

This project follows the Posit Code of Conduct. By participating, you agree to uphold this code. Please report unacceptable behavior to the project maintainers.

## Reporting issues

We welcome and encourage issue reports! This is one of the most valuable ways to contribute.

### Bug reports

Include:

1. **Go version**: `go version`
2. **SDK version**: Git commit or tag
3. **Operating system**: Linux, macOS
4. **Description**: What happened vs what you expected
5. **Reproduction**: Minimal code to reproduce
6. **Logs**: Relevant log output

Use the bug report template on GitHub.

### Feature requests

Include:

1. **Use case**: What problem does this solve?
2. **Proposed solution**: How should it work?
3. **Alternatives**: Other approaches you considered
4. **Additional context**: Any relevant information

Use the feature request template on GitHub.

### Security issues

Do not open public issues for security vulnerabilities.

Instead, email security@posit.co with:
- Description of the vulnerability
- Steps to reproduce
- Potential impact

## Development setup

1. **Fork the repository** on GitHub

2. **Clone your fork**:
```bash
git clone https://github.com/YOUR-USERNAME/launcher-go-sdk.git
cd launcher-go-sdk
```

3. **Add upstream remote**:
```bash
git remote add upstream https://github.com/posit-dev/launcher-go-sdk.git
```

4. **Install dependencies**:
```bash
go mod download
```

5. **Install development tools** (optional but recommended):
```bash
just install-tools          # golangci-lint, goimports
brew install difftastic yq  # structural diffs, YAML processing
```

This installs `golangci-lint`, `goimports`, and other development tools. `difftastic` provides syntax-aware diffs for Go, and `yq` is a YAML processor useful for working with CI and linter configs.

6. **Verify setup**:
```bash
just build
just test
```

Or without `just`:
```bash
go build ./...
go test ./...
```

## Making changes

### Creating a branch

Create a branch for your work:

```bash
git checkout -b feature/my-new-feature
# or
git checkout -b fix/issue-123
```

Branch naming conventions:
- `feature/description` - New features
- `fix/description` - Bug fixes
- `docs/description` - Documentation changes
- `test/description` - Test improvements

### Development workflow

This project uses [`just`](https://github.com/casey/just) as a command runner. See all available commands with:

```bash
just --list
```

Common development commands:

| Command | Description |
|---------|-------------|
| `just test` | Run all tests |
| `just test-coverage` | Run tests with coverage report |
| `just lint` | Run linter |
| `just fmt` | Format all code |
| `just build` | Build all packages |
| `just check` | Run format check, lint, and tests |
| `just pre-commit` | Quick checks before committing |
| `just pre-push` | Comprehensive checks before pushing |

#### Step-by-step workflow:

1. **Make your changes** in your branch

2. **Write tests** for your changes

3. **Run pre-commit checks** (fast):
```bash
just pre-commit
```

Or manually:
```bash
just fmt       # Format code
just lint      # Run linter
```

4. **Run tests** to ensure everything works:
```bash
just test
```

5. **Commit your changes** with clear messages

7. **Push to your fork**:
```bash
git push origin feature/my-new-feature
```

### Keeping your fork updated

Regularly sync with upstream:

```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

## Testing

Include tests with all contributions.

### Running tests

```bash
# All tests
go test ./...

# Specific package
go test ./launcher

# With coverage
go test ./... -cover

# Verbose output
go test ./... -v
```

### Writing tests

- Use table-driven tests for multiple scenarios
- Test both success and failure cases
- Use the `plugintest` package for plugin testing
- Keep tests fast (use short timeouts)
- Use meaningful test names

Example:

```go
func TestNewFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "TEST", false},
        {"empty input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := NewFeature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("NewFeature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("NewFeature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Test coverage

- Aim for 80%+ coverage on new code
- 100% coverage of error paths
- Focus on critical paths, not just line coverage

## Documentation

### Code documentation

- All exported types and functions must have godoc comments
- Start comments with the name of the item
- Provide examples for complex functionality

Example:

```go
// JobCache provides thread-safe in-memory job storage with pub/sub for status
// updates. Users can only access their own jobs, and the cache automatically
// removes jobs after the configured expiration period. The scheduler is the
// source of truth; plugins should populate the cache during Bootstrap() and
// keep it in sync via periodic polling.
type JobCache struct {
    // ...
}

// NewJobCache creates a new in-memory job cache instance.
//
// The cache starts background goroutines for pub/sub management. These
// goroutines are stopped when ctx is cancelled.
func NewJobCache(ctx context.Context, lgr *slog.Logger) (*JobCache, error) {
    // ...
}
```

### Documentation files

Update relevant documentation when making changes:

- `README.md` - For major features
- `docs/GUIDE.md` - For user-facing changes
- `docs/API.md` - For API changes
- `docs/TESTING.md` - For test utilities
- `docs/ARCHITECTURE.md` - For design decisions
- `CHANGELOG.md` - All changes

### Examples

When adding new features, consider adding examples:

```go
func ExampleJobBuilder() {
    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("python train.py").
        WithMemory("8GB").
        Build()

    fmt.Println(job.User)
    // Output: alice
}
```

## Pull request process

### Before submitting

1. ✅ Tests pass: `go test ./...`
2. ✅ Code is formatted: `go fmt ./...`
3. ✅ Documentation is updated
4. ✅ CHANGELOG.md is updated
5. ✅ Commit messages are clear
6. ✅ Branch is up to date with main

### Submitting a PR

1. **Push your branch** to your fork

2. **Create a pull request** on GitHub

3. **Fill out the PR template**:
   - Describe what changed and why
   - Link related issues
   - Note any breaking changes
   - Add screenshots/examples if relevant

4. **Request review** from maintainers

### PR title format

Use conventional commit format:

- `feat: add support for custom schedulers`
- `fix: resolve race condition in job cache`
- `docs: improve testing guide`
- `test: add tests for streaming responses`
- `refactor: simplify protocol handling`

### Review process

1. Automated checks run (tests, linting)
2. Maintainers review your code
3. Address feedback by pushing new commits
4. Decision on whether the PR aligns with project goals
5. Approval from at least one maintainer (if accepted)
6. Merge by maintainers

Please note that we may not accept even well-crafted PRs if they don't align with the current project direction, introduce maintenance burden, or overlap with planned work. We'll do our best to provide clear feedback on the decision.

## Code style

### Go style

Follow standard Go conventions:

- Use `gofmt` for formatting
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Specific guidelines

**Naming**:
- Use `MixedCaps` or `mixedCaps` (not underscores)
- Short variable names for short scopes: `i`, `w`, `lgr`
- Longer names for longer scopes: `responseWriter`, `jobCache`
- Interface names: `Reader`, `Writer`, `Plugin`

**Error Handling**:
```go
// Good
if err != nil {
    return fmt.Errorf("failed to submit job: %w", err)
}

// Bad
if err != nil {
    panic(err)
}
```

**Comments**:
```go
// Good - explains why
// We use a buffered channel here to prevent blocking when the
// subscriber is slow to process updates.
ch := make(chan *api.Job, 10)

// Bad - explains what (code already shows this)
// Create a channel
ch := make(chan *api.Job, 10)
```

**Structs**:
```go
// Good - fields are documented
type Config struct {
    // Address is the host:port to bind to
    Address string

    // Timeout is the maximum duration for requests
    Timeout time.Duration
}

// Bad - no documentation
type Config struct {
    Address string
    Timeout time.Duration
}
```

## Commit messages

### Format

```
type(scope): subject

body

footer
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Test changes
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `chore`: Build/tooling changes

### Examples

```
feat(cache): add support for Redis backend

Add a new Redis-backed storage implementation for the job cache.
This allows multiple plugin instances to share job state.

Closes #123
```

```
fix(protocol): handle large message payloads correctly

The protocol reader was not handling messages larger than 5MB.
Increase the buffer size and add proper error handling.

Fixes #456
```

### Guidelines

- **First line**: 50 characters or less
- **Body**: Wrap at 72 characters
- **Use imperative mood**: "add" not "added" or "adds"
- **Explain what and why**, not how
- **Reference issues**: "Fixes #123" or "Closes #456"


## API stability

### Current status: Pre-1.0 (v0.x)

During v0.x:
- Minor versions (v0.1 → v0.2) may include breaking changes
- Breaking changes will have migration guides
- CHANGELOG will clearly mark breaking changes

### After v1.0

Following [Semantic Versioning](https://semver.org/):
- Major version (v1 → v2): Breaking changes allowed
- Minor version (v1.0 → v1.1): New features, backwards compatible
- Patch version (v1.0.0 → v1.0.1): Bug fixes, backwards compatible

## License

By contributing, you agree to license your contributions under the project's [LICENSE](LICENSE).

## Questions?

- **GitHub Discussions**: For general questions
- **GitHub Issues**: For bugs and feature requests
- **Posit Community**: https://forum.posit.co/

Thank you for your interest in the Launcher Go SDK!
