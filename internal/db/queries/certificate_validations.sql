-- name: GetCertificateValidation :one
SELECT * FROM certificate_validations
WHERE hostname = ? AND certificate_hash = ?
AND (expires_at IS NULL OR expires_at > datetime('now'))
LIMIT 1;

-- name: GetCertificateValidationByHostname :one
SELECT * FROM certificate_validations
WHERE hostname = ?
AND (expires_at IS NULL OR expires_at > datetime('now'))
ORDER BY created_at DESC
LIMIT 1;

-- name: StoreCertificateValidation :exec
INSERT OR REPLACE INTO certificate_validations (hostname, certificate_hash, user_decision, expires_at)
VALUES (?, ?, ?, ?);

-- name: DeleteExpiredCertificateValidations :exec
DELETE FROM certificate_validations 
WHERE expires_at IS NOT NULL AND expires_at <= datetime('now');

-- name: DeleteCertificateValidation :exec
DELETE FROM certificate_validations 
WHERE hostname = ? AND certificate_hash = ?;