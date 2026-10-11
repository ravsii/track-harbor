# TrackHarbor Agent Instructions

## Project Goal

The core goal is to synchronize and store collections of tracks, then download
them for local use. The primary use case is keeping local copies of Spotify
playlists or YouTube video/audio without requiring a subscription.

## MVP: build fast, fail fast

- We are building an MVP. Prioritize getting the core workflow working quickly
  over designing a universal architecture or adding speculative features.
- When a request is clear, proceed directly to implementation. Do not create
  preliminary drafts, specifications, design documents, or implementation plans
  unless explicitly requested. Do not require separate approval of such documents.
- Do not write or run tests, including unit, integration, race, and live tests,
  unless explicitly requested. Do not delete existing tests.
- Keep changes small, simple, and focused on the task. Follow the existing code
  structure and style; avoid unnecessary abstractions and unrelated refactoring.
- Report errors with clear explanations so problems can be fixed quickly and
  development can continue.
- Keep final responses concise and describe what changed. Do not claim that code
  was verified or tests passed when those checks were not performed.

During the MVP phase, these rules override default design, planning, and
test-driven development workflows.
