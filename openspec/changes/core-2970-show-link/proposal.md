# Resolve Apple Podcasts show links in the plugin (CORE-2970)

## Why

ADR-0238 §2.5 lets a user paste an Apple Podcasts show page into feed search. Core must stay free of
podcast vocabulary, so recognising the Apple hostname and show ID belongs in this plugin.

## What Changes

- Add `ResolveShowLink`, which turns an Apple Podcasts show URL into a keyless iTunes lookup through
  the caller's `Doer`, so the request shares Core's SSRF gate, request budget and response cap.
- A non-Apple link is unmatched; an Apple page without a numeric show ID matches with no request.

## Impact

- `directory.go`; Core CORE-2970 calls it from its compiled feed-search hook.
