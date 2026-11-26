# Show containers and a Shows shelf (CORE-3524)

Core now publishes one `show` container Entity per subscription (core change
`feed-show-containers`), holding episodes through a single-holder Media relation, and inserts
the catalogue newest-last so `recently-added` lists the newest episode first.

This plugin declares the holder relation `episode` on `show` and replaces the flat "All Episodes"
grid on the home layout with a Shows shelf backed by the `show` entities. Requires a core build
carrying CORE-3524; on older cores the relation is unused and the shelf is empty.
