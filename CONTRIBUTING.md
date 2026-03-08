# Contributing to Navi

Thank you for considering contributing to Navi! This document provides guidelines and information for contributors.

## Code of Conduct

Navi is committed to a harassment-free environment. By participating, you agree to:

- Be respectful
- Accept constructive feedback
- Focus on what's best for the community

Harassment or discrimination in any form will not be tolerated.

## Getting Started

### Prerequisites

- Go 1.25 or higher
- Git
- Docker (for integration tests)
- Make (optional)

### Setting Up Development Environment

1. **Fork and clone**
   ```bash
   git clone https://github.com/enrell/navi.git
   cd navi
   ```

2. **Install dependencies**
   ```bash
   go mod download
   go mod tidy
   ```

3. **Build and test**
   ```bash
   make build
   make test
   ```

   Or manually:
   ```bash
   go build ./...
   go test ./...
   ```

 4. **Run the REST API server**
    ```bash
    ./navi serve
    # or
    go run cmd/navi/main.go serve
    ```

    **Alternative interfaces:**
    ```bash
    ./navi repl         # Full-screen terminal UI (or use --plain for line REPL)
    ./navi tui          # Explicit Bubble Tea TUI entry point
    ./navi chat <msg>   # Single chat message
    ```

### Development Tools

Recommended:
- **Editor**: VS Code with Go extension, or GoLand, or vim/neovim with gopls
- **Linting**: `golangci-lint` (install: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)
- **Formatting**: `gofmt` (built-in) or `goimports`

## Development Workflow

### 1. Find an Issue

Look for issues labeled `good-first-issue` or `help-wanted` on GitHub: https://github.com/enrell/navi/issues

If you have a new feature idea, open an issue first to discuss it.

### 2. Create a Branch

```bash
git checkout -b feat/your-feature-name
# or
git checkout -b fix/issue-number-description
```

Branch naming:
- `feat/` for new features
- `fix/` for bug fixes
- `docs/` for documentation changes
- `refactor/` for code reorganization
- `test/` for test additions

### 3. Make Changes

Follow these guidelines:

- **Keep it small**: One change per PR
- **Write tests**: Every new feature needs tests
- **Follow existing patterns**: Look at similar code
- **Document**: Add godoc comments for exported items
- **Error handling**: Return errors, don't panic
- **Security first**: Validate inputs, enforce capabilities

### 4. Run Tests

```bash
# All tests
go test ./...

# Verbose
go test -v ./...

# With race detection
go test -race ./...

# Lint
golangci-lint run
```

All tests must pass before submitting PR.

### 5. Commit

```bash
git add .
git commit -m "feat: add new isolation adapter for firecracker"
```

Commit message format ( Conventional Commits ):

```
type(scope): subject

body (optional)

footer (optional)
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Example:

```
feat(isolation): add Firecracker microVM adapter

- Implement FirecrackerAdapter with configurable VM size
- Add seccomp filter for syscall restriction
- Support snapshotting for fast startup

Closes #123
```

### 6. Push and Open PR

```bash
git push origin feat/your-feature-name
```

Then open a Pull Request on GitHub with:
- Clear description of what changed and why
- Reference to related issues (Closes #123)
- Screenshots for UI changes
- Testing instructions

## Coding Standards

### Go Style

- Use `gofmt` (should be automatic in your editor)
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `golangci-lint` and fix all warnings

### Error Handling

```go
// Good
if err != nil {
    return fmt.Errorf("failed to do thing: %w", err)
}

// Bad
if err != nil {
    return err  // Unwrapped, loses context
}
```

### Context Usage

```go
func (s *Service) DoThing(ctx context.Context, arg string) error {
    // Always accept context as first parameter
    // Pass it to all downstream calls
    return s.repo.Update(ctx, arg)
}
```

### Interface Design

- Name interfaces with `-er` suffix if they describe behavior (`Reader`, `Writer`, `Executor`)
- Keep interfaces small (single method is ideal)
- Accept interfaces, return structs

```go
// Good
type Store interface {
    Get(ctx context.Context, id string) (Item, error)
}

// Accept interface
func NewService(store Store) *Service {
    return &Service{store: store}
}
```

### Comments and Documentation

- Every exported function, type, method needs a comment
- Use complete sentences ending with periods
- Document **why**, not **what** (the code shows what)
- Example:

```go
// Execute runs the given command in the isolated environment.
// It enforces capability constraints and returns the command output.
func (a *Adapter) Execute(ctx context.Context, cmd Command) (Result, error) {
    // ...
}
```

## Testing

### Unit Tests

- Place in `*_test.go` files in the same package
- Use table-driven tests for multiple cases
- Mock dependencies; isolate unit under test

Example:

```go
func NewTest(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid", "input", "output", false},
        {"error", "bad", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := process(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("process error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("process = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

Use `testing.Short()` to skip integration tests in quick runs:

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    // Test that hits real Docker/bwrap...
}
```

Run with `go test -v ./...` to include all tests.

### Benchmarks

If performance matters, add benchmarks:

```go
func BenchmarkExecute(b *testing.B) {
    a := NewAdapter()
    for i := 0; i < b.N; i++ {
        a.Execute(context.Background(), Command{Args: []string{"echo", "hi"}})
    }
}
```

## Areas Needing Help

Current priorities (check [ROADMAP.md](../ROADMAP.md)):

1. **Complete OpenAI adapter** (`internal/adapters/openai_adapter.go:23` - unused `payload` variable, typo `promtp`, context param mismatch)
2. **Implement storage layer** (SQLite repository for event log)
3. **Basic authentication** (local token auth)
4. **Docker adapter** (not created yet)
5. **Bubblewrap adapter** (not created yet)
6. **Orchestrator** (core coordination logic)
7. **Agent implementations** (only placeholders exist)
8. **REST API** (Chi router setup)
9. **Comprehensive tests** (currently none exist)
10. **Documentation** (examples, tutorials)

## Pull Request Process

1. Ensure tests pass and linting is clean
2. Update documentation if needed (README, docs/, comments)
3. Open PR with clear description
4. Respond to review feedback
5. Squash and merge (maintainer will handle)

### What Maintainers Look For

- ✅ Tests included (or rationale why not needed)
- ✅ Follows coding standards
- ✅ Documentation updated
- ✅ Issue referenced (if applicable)
- ✅ Small, focused change (not a giant refactor)
- ✅ No unrelated changes

## Review Process

- Maintainers will review within a few days
- Be responsive to feedback
- Make requested changes or discuss alternatives
- Once approved, a maintainer will merge

## Security Reporting

**Do not open GitHub issues for security vulnerabilities.** Email security@enrell.dev with:
- Description of vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We aim to respond within 48 hours and patch within 30 days.

## Questions?

- **Discord**: https://discord.gg/eNsMFGZU (fastest response)
- **GitHub Issues**: For bugs and feature requests
- **Documentation**: Check [docs/](.) first

## Recognition

Contributors will be:
- Listed in [CREDITS.md](../CREDITS.md) (to be created)
- Mentioned in release notes
- Given deep appreciation from the community ❤️

---

Thank you for helping make Navi better!
