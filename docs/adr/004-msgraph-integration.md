# ADR 004: Microsoft Graph Integration for Teams Chats

## Status

Accepted

## Context

Section 5 of jirlab is being repurposed from "Time Tracker" to "Chats". The primary goal is to surface the user's pinned/favorited Microsoft Teams conversations directly in the TUI, enabling quick reference without switching applications.

Microsoft Graph is the only supported API for accessing Teams conversations. Authentication requires OAuth 2.0, which presents challenges in a terminal environment.

## Decision

### Authentication: OAuth 2.0 Device Code Flow

We use the **device code flow** (RFC 8628) with the public Azure CLI client ID (`04b07795-8ddb-461a-bbee-02f9e1bf7b46`). This requires no client secret and is the standard approach for native/CLI applications.

- On first run (no token cached), a device code is requested from `https://login.microsoftonline.com/common/oauth2/v2.0/devicecode`.
- The user code is copied to clipboard and the browser is opened to `https://login.microsoft.com/device`.
- The user confirms in the TUI after authenticating in the browser.
- The access token (and refresh token if issued) is stored in `~/.jirlab/msgraph_token.json`.
- Subsequent runs load the cached token; re-auth is only required on expiry.

### Hexagonal Architecture

The integration is strictly layered:

- **`internal/integration/msgraph.go`** — `MSGraphService` interface (the port)
- **`internal/service/msgraph_client.go`** — `MSGraphClient` concrete implementation (the adapter)
- **`internal/tui/chats.go`** — `ChatsSection` TUI, depends only on `MSGraphService` interface

The TUI never calls `net/http` or token endpoints directly.

### Section 5 Restructure

- Section 5 is renamed from "Time Tracker" to "Chats".
- Top table: pinned/favorited Teams chats (from Microsoft Graph).
- Bottom table: time track notes and templates (moved from TrackerSection).
- Time worklogs remain accessible via the Sprint Board (`l` key).
- Tab navigation within section 5: `tab` cycles top↔bottom (not across sections).

### Token Storage

Token stored as JSON at `~/.jirlab/msgraph_token.json`. This file is created only with user-mode permissions (0600). It is never committed to version control.

### Scope

`https://graph.microsoft.com/.default offline_access openid profile`

This requests the broadest Graph scope using `.default`, which reflects the app's configured API permissions. `offline_access` enables refresh token issuance.

### Chats Endpoint

`GET https://graph.microsoft.com/v1.0/me/chats?$filter=viewpoint/isPinned eq true`

This returns only pinned chats for the authenticated user. The Graph API does not expose a "favorites" concept separately from pinned.

## Alternatives Considered

### Silent token acquisition (MSAL / azidentity)

The `azidentity` SDK supports managed identity and workload identity, but requires an app registration with credentials. Using the public client ID avoids the need for each user to register an app or configure secrets.

### Browser-based redirect (authorization code flow)

Not feasible in a TUI context — there is no localhost redirect URI available.

### Environment variable for access token

Rejected. Token expiry would cause silent failures and env vars are insecure for long-lived tokens.

## Consequences

- First-run requires browser interaction (unavoidable for delegated auth without a registered app).
- Token expiry (typically ~1 hour) requires periodic re-auth; refresh tokens may extend this silently.
- The integration is fully testable via mock `MSGraphService`; no real network calls in tests.
- Future: adding support for sending messages or creating chats can be done by extending `MSGraphService`.
