package usecase_test

import (
	"context"

	"bnema.dev/dumber/internal/logging"
)

func testContext() context.Context {
	logger := logging.NewFromConfigValues("debug", "console")
	return logging.WithContext(context.Background(), logger)
}
