# TrackHarbor Agent Instructions

## Project Goal

The core goal is to synchronize and store collections of tracks, then download
them for local use. The primary use case is keeping local copies of Spotify
playlists or YouTube video/audio without requiring a subscription.

## Project boundaries

- The server is the only supported application. Its Go module, command,
  configuration, database, and source integrations live at the repository root.
  The old CLI is archived in `.deprecated/cli/` and is not maintained or built.
  Never point the server at the CLI's `~/.trackharbor/vault.db`.
- The web UI has no user login yet: bind it to loopback by default and do not
  expose it publicly without access control. Multi-device access needs login.

## Development

- Before changing the server web UI's visual elements, read
  `docs/visual-design.md` and follow its palette and light/dark theme
  requirements for every new or changed element.
- Prioritize the core workflow over speculative features, but keep application
  boundaries explicit and deployment/security constraints visible.
- When a request is clear, proceed directly to implementation. Do not create
  preliminary drafts, specifications, design documents, or implementation plans
  unless explicitly requested. Do not require separate approval of such documents.
- If a plan is requested or needed, present it in the conversation only.
  Do not save plans in `docs/` or any other project file.
- Do not write or run tests, including unit, integration, race, and live tests,
  unless explicitly requested. Do not delete existing tests.
- Keep changes small, simple, and focused on the task. Follow the existing code
  structure and style; avoid unnecessary abstractions and unrelated refactoring.
- Never delete or edit existing database migrations. If a migration is wrong,
  correct or reverse it with a new migration instead.
- Report errors with clear explanations so problems can be fixed quickly and
  development can continue.
- Server logs use structured JSON via zap with Info as the default minimum
  level. Never add Debug log statements: if an event does not belong in Info
  (or Warn/Error), do not log it. Keep polling, transfer progress, and other
  noisy events out of logs. Never log credentials, tokens, proxy URLs, request
  or response bodies, or upstream errors that may contain secrets.
- Keep final responses concise and describe what changed. Do not claim that code
  was verified or tests passed when those checks were not performed.

These project rules take precedence over default planning and test-driven
development workflows unless explicitly updated by the user.

## Commits

- Use Conventional Commits with a required component scope for every commit:
  `type(component): message`. Choose the component affected by the change,
  such as `server`, `cli`, or `common` for shared code. Examples:
  `feat(server): add top-bar`, `feat(common): better proxy logic`,
  `fix(vk): skip tracks without audio URLs`. See `CONTRIBUTING.md`.
- When asked to create a commit, keep the human as the primary author and add
  the assistant as a co-author using a `Co-authored-by:` commit-message trailer.
