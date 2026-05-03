# Copilot Instructions for go-fim

## Commit Message Format

All commit messages MUST follow **Conventional Commits** format for GoReleaser changelog generation.

### Format

```
<type>(<scope>): <short description>

[optional body]
```

### Types

| Type       | Description                          | Changelog   |
|------------|--------------------------------------|-------------|
| `feat`     | New feature                          | ✅ Included |
| `fix`      | Bug fix                              | ✅ Included |
| `refactor` | Code refactoring                     | ✅ Included |
| `perf`     | Performance improvement              | ✅ Included |
| `build`    | Build system changes                 | ✅ Included |
| `ci`       | CI configuration                     | ✅ Included |
| `docs`     | Documentation only                   | ❌ Excluded |
| `test`     | Adding/updating tests                | ❌ Excluded |
| `chore`    | Maintenance tasks                    | ❌ Excluded |

### Scopes (optional)

Use component names: `walker`, `hasher`, `store`, `report`, `client`, `config`, `server`, `logger`

### Breaking Changes

Add `!` after type for major version bumps:

```
feat!: remove deprecated API
```

### Examples

- `feat(walker): add recursive directory scanning`
- `fix(hasher): correct SHA256 computation for large files`
- `refactor(report): simplify JSON encoding`
- `docs: update README with installation instructions`
- `chore: update dependencies`
