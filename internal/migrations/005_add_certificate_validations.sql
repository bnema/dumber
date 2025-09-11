-- Add certificate validation storage for permanent TLS decisions
-- Store user decisions for certificate errors to avoid repeated prompts

CREATE TABLE IF NOT EXISTS certificate_validations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname TEXT NOT NULL,
    certificate_hash TEXT NOT NULL,
    user_decision TEXT NOT NULL CHECK (user_decision IN ('accepted', 'rejected')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME, -- Optional expiration for temporary decisions
    UNIQUE(hostname, certificate_hash)
);

CREATE INDEX idx_cert_validations_hostname ON certificate_validations(hostname);
CREATE INDEX idx_cert_validations_hash ON certificate_validations(certificate_hash);
CREATE INDEX idx_cert_validations_expires ON certificate_validations(expires_at);