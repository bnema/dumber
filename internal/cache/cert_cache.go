package cache

import (
	"context"

	"github.com/bnema/dumber/internal/cache/generic"
	"github.com/bnema/dumber/internal/db"
)

// CertDBOperations implements DatabaseOperations for certificate validations cache.
// Handles loading, persisting, and deleting certificate validations from the database.
type CertDBOperations struct {
	queries db.DatabaseQuerier
}

// NewCertDBOperations creates a new CertDBOperations instance.
func NewCertDBOperations(queries db.DatabaseQuerier) *CertDBOperations {
	return &CertDBOperations{
		queries: queries,
	}
}

// LoadAll loads all certificate validations from the database.
// Returns a map of hostname -> CertificateValidation.
// Only loads non-expired validations.
func (c *CertDBOperations) LoadAll(ctx context.Context) (map[string]db.CertificateValidation, error) {
	validations, err := c.queries.ListCertificateValidations(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]db.CertificateValidation, len(validations))
	for _, validation := range validations {
		// Use hostname as key - most recent validation per hostname
		// (query already orders by created_at DESC in GetCertificateValidationByHostname)
		result[validation.Hostname] = validation
	}

	return result, nil
}

// Persist saves a certificate validation to the database.
// Uses INSERT OR REPLACE to handle both new and existing entries.
func (c *CertDBOperations) Persist(ctx context.Context, hostname string, validation db.CertificateValidation) error {
	return c.queries.StoreCertificateValidation(
		ctx,
		hostname,
		validation.CertificateHash,
		validation.UserDecision,
		validation.ExpiresAt,
	)
}

// Delete removes a certificate validation from the database.
func (c *CertDBOperations) Delete(ctx context.Context, hostname string) error {
	// Get the validation to find the certificate hash
	validation, err := c.queries.GetCertificateValidationByHostname(ctx, hostname)
	if err != nil {
		return err
	}

	return c.queries.DeleteCertificateValidation(ctx, hostname, validation.CertificateHash)
}

// CertValidationsCache is a specialized cache for certificate validations.
// It wraps GenericCache with hostname-specific helper methods.
// Key insight: Caches hostname -> validation decision to avoid DB queries on HTTPS cert errors.
type CertValidationsCache struct {
	*generic.GenericCache[string, db.CertificateValidation]
}

// NewCertValidationsCache creates a new certificate validations cache.
func NewCertValidationsCache(queries db.DatabaseQuerier) *CertValidationsCache {
	dbOps := NewCertDBOperations(queries)
	return &CertValidationsCache{
		GenericCache: generic.NewGenericCache(dbOps),
	}
}
