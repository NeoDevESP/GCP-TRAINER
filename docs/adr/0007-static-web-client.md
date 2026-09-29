# ADR 0007 — Static Next.js client served by the Go binary

- Status: accepted
- Context: the UI needs a terminal (xterm.js), an editor (Monaco) and diagrams
  (Mermaid); production should not need a Node runtime.
- Decision: Next.js with `output: "export"` builds static pages into
  `web/out`, served by the API with clean URLs (`/lab` → `lab.html`). Pages
  read query parameters (`/lab?session=…`) instead of dynamic routes. The
  client only calls the JSON API, so the terminal and the console views share
  the same state.
- Consequences: one container serves everything; a CDN can host the client
  separately with `CORS_ORIGIN`. Server-side rendering is not available, which
  is acceptable for an authenticated app.
