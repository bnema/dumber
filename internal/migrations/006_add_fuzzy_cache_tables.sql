-- Database consolidation: Move fuzzy cache from binary file to database
-- Replaces ~/.local/state/dumber/dmenu_fuzzy_cache.bin with proper SQL tables
-- This enables single-database architecture: dumber.sqlite

-- Metadata table: stores cache statistics and versioning
CREATE TABLE IF NOT EXISTS fuzzy_cache_metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1), -- Singleton table: only one row allowed
    version INTEGER NOT NULL,
    entry_count INTEGER NOT NULL,
    last_modified INTEGER NOT NULL, -- Unix timestamp
    entries_hash TEXT NOT NULL      -- MD5 hash for cache invalidation detection
);

-- Structures table: stores serialized fuzzy search data structures
CREATE TABLE IF NOT EXISTS fuzzy_cache_structures (
    id INTEGER PRIMARY KEY CHECK (id = 1), -- Singleton table: only one row allowed
    trigram_index BLOB NOT NULL,           -- Serialized trigram index: map[string][]int
    prefix_trie BLOB NOT NULL,             -- Serialized prefix trie for fast lookups
    sorted_index BLOB NOT NULL             -- Pre-sorted entries for ranking
);

-- Note: No indexes needed - singleton tables with single row access pattern
