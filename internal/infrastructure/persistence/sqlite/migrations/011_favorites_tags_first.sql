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
), resolved AS (
    SELECT
        id,
        CASE
            WHEN base_slug = '' THEN 'folder-' || id
            WHEN EXISTS (
                SELECT 1 FROM favorite_tags t
                WHERE lower(trim(t.name)) = lower(trim(base_slug))
            ) OR EXISTS (
                SELECT 1 FROM folder_slugs earlier
                WHERE earlier.id < folder_slugs.id
                  AND lower(trim(earlier.base_slug)) = lower(trim(folder_slugs.base_slug))
            ) THEN base_slug || '-folder-' || id
            ELSE base_slug
        END AS tag_name
    FROM folder_slugs
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
