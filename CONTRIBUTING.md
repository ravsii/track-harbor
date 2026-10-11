# Contributing

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
messages:

```text
<type>(<optional scope>): <short description>
```

Use `feat` for new functionality, `fix` for bug fixes, and `docs`, `refactor`,
`chore`, or `perf` when appropriate. For breaking changes, add `!` after the
type or scope and describe the change in the commit body.

Examples:

```text
feat(vk): download tracks directly with ffmpeg
fix(spotify): keep proxy-only downloads from using direct connections
docs: describe configuration options
```
