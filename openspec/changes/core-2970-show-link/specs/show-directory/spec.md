## ADDED Requirements

### Requirement: Show link resolution

The plugin SHALL resolve an Apple Podcasts show page link to its directory entries with one keyless
iTunes lookup made through the caller's fetcher, and SHALL report links on other hosts as unmatched.

#### Scenario: Show page with a numeric ID

- **WHEN** the caller resolves `https://podcasts.apple.com/ca/podcast/daily/id1234567`
- **THEN** the plugin fetches `https://itunes.apple.com/lookup?country=ca&id=1234567` through the
  caller's fetcher and returns the entries with safe feed URLs

#### Scenario: Apple page without a show ID

- **WHEN** the caller resolves an Apple Podcasts page whose last path segment is not `id<digits>`
- **THEN** the link is matched, no entries are returned and no request is made

#### Scenario: Link on another host

- **WHEN** the caller resolves a link whose host is not `podcasts.apple.com`
- **THEN** the link is unmatched and no request is made
