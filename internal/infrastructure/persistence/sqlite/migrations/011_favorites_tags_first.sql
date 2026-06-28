-- +goose Up
-- Convert favorite organization from folders to tags.

-- Reconcile duplicate tags by folded name before adding folded uniqueness.
CREATE TEMP TABLE _favorite_tag_canonical AS
SELECT lower(trim(name)) AS folded_name, MIN(id) AS canonical_id
FROM favorite_tags
GROUP BY lower(trim(name));

INSERT OR IGNORE INTO favorite_tag_assignments (favorite_id, tag_id)
SELECT fta.favorite_id, c.canonical_id
FROM favorite_tag_assignments fta
JOIN favorite_tags t ON t.id = fta.tag_id
JOIN _favorite_tag_canonical c ON c.folded_name = lower(trim(t.name))
WHERE fta.tag_id <> c.canonical_id;

DELETE FROM favorite_tag_assignments
WHERE tag_id NOT IN (SELECT canonical_id FROM _favorite_tag_canonical);

DELETE FROM favorite_tags
WHERE id NOT IN (SELECT canonical_id FROM _favorite_tag_canonical);

DROP TABLE _favorite_tag_canonical;

CREATE UNIQUE INDEX IF NOT EXISTS idx_favorite_tags_name_folded ON favorite_tags(lower(trim(name)));

-- Build deterministic folder-path slugs. Keep folder tables/columns for compatibility,
-- but materialize every current folder assignment as a tag assignment.
CREATE TEMP TABLE _folder_tag_map (
    folder_id INTEGER PRIMARY KEY,
    tag_name TEXT NOT NULL UNIQUE
);

WITH RECURSIVE folder_paths(id, parent_id, path) AS (
    SELECT id, parent_id, trim(name)
    FROM favorite_folders
    WHERE parent_id IS NULL
    UNION ALL
    SELECT child.id, child.parent_id, folder_paths.path || '-' || trim(child.name)
    FROM favorite_folders child
    JOIN folder_paths ON child.parent_id = folder_paths.id
), folder_slugs AS (
    SELECT
        id,
        lower(
            trim(
                replace(replace(replace(replace(replace(path, '/', '-'), '\\', '-'), ' ', '-'), '_', '-'), '.', '-'),
                '-'
            )
        ) AS base_slug
    FROM folder_paths
), folder_names AS (
    SELECT
        id,
        CASE
            WHEN base_slug = '' THEN 'folder-' || id
            ELSE base_slug
        END AS natural_name
    FROM folder_slugs
), base_assignments AS (
    SELECT id, natural_name AS tag_name
    FROM folder_names current_folder
    WHERE NOT EXISTS (
        SELECT 1 FROM favorite_tags t
        WHERE lower(trim(t.name)) = lower(trim(current_folder.natural_name))
    )
      AND NOT EXISTS (
        SELECT 1 FROM folder_names earlier
        WHERE earlier.id < current_folder.id
          AND lower(trim(earlier.natural_name)) = lower(trim(current_folder.natural_name))
    )
), unresolved_folders AS (
    SELECT id, natural_name
    FROM folder_names
    WHERE id NOT IN (SELECT id FROM base_assignments)
), candidate_limit(max_attempt) AS (
    SELECT (SELECT COUNT(*) FROM favorite_tags) + (SELECT COUNT(*) FROM folder_names) + 3
), candidate_numbers(attempt) AS (
    SELECT 1
    UNION ALL
    SELECT attempt + 1
    FROM candidate_numbers, candidate_limit
    WHERE attempt < max_attempt
), suffix_candidates AS (
    SELECT
        unresolved_folders.id,
        candidate_numbers.attempt,
        CASE
            WHEN candidate_numbers.attempt = 1 THEN unresolved_folders.natural_name || '-folder-' || unresolved_folders.id
            ELSE unresolved_folders.natural_name || '-folder-' || unresolved_folders.id || '-' || candidate_numbers.attempt
        END AS tag_name
    FROM unresolved_folders
    CROSS JOIN candidate_numbers
), available_suffix_candidates AS (
    SELECT id, attempt, tag_name
    FROM suffix_candidates candidate
    WHERE NOT EXISTS (
        SELECT 1 FROM favorite_tags t
        WHERE lower(trim(t.name)) = lower(trim(candidate.tag_name))
    )
      AND NOT EXISTS (
        SELECT 1 FROM base_assignments assigned
        WHERE lower(trim(assigned.tag_name)) = lower(trim(candidate.tag_name))
    )
), suffix_assignments AS (
    SELECT id, tag_name
    FROM available_suffix_candidates candidate
    WHERE attempt = (
        SELECT MIN(attempt)
        FROM available_suffix_candidates earlier
        WHERE earlier.id = candidate.id
    )
), resolved AS (
    SELECT id, tag_name FROM base_assignments
    UNION ALL
    SELECT id, tag_name FROM suffix_assignments
)
INSERT INTO _folder_tag_map(folder_id, tag_name)
SELECT id, tag_name FROM resolved;

INSERT OR IGNORE INTO favorite_tags(name, color)
SELECT tag_name, '#808080'
FROM _folder_tag_map;

INSERT OR IGNORE INTO favorite_tag_assignments(favorite_id, tag_id)
SELECT f.id, t.id
FROM favorites f
JOIN _folder_tag_map ftm ON ftm.folder_id = f.folder_id
JOIN favorite_tags t ON lower(trim(t.name)) = lower(trim(ftm.tag_name))
WHERE f.folder_id IS NOT NULL;

DROP TABLE _folder_tag_map;

-- +goose Down
DROP INDEX IF EXISTS idx_favorite_tags_name_folded;
