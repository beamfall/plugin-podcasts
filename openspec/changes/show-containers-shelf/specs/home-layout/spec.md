## ADDED Requirements

### Requirement: Show holder relation
The `show` kind SHALL declare an `episode` relation to Media with cardinality `one`.

#### Scenario: bundle validates
- **WHEN** the bundle is validated
- **THEN** `show` carries `episode` (ToKind media, Cardinality one)

### Requirement: Shows shelf on home
The home layout SHALL present a Shows shelf sourced from `entity-items` of kind `show` in place
of the flat All Episodes grid, after Latest Episodes and Continue Listening.

#### Scenario: no flat episode grid
- **WHEN** the home layout is read
- **THEN** it has a `shows` section and no `all-episodes` section
