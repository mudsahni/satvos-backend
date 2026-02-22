// Command backfill-normalize-tags normalizes existing seller_name and buyer_name
// auto-tags to the canonical UPPER CASE form.
// Usage: go run ./cmd/backfill-normalize-tags
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"satvos/internal/config"
	"satvos/internal/normalize"
	"satvos/internal/repository/postgres"
)

const batchSize = 500

type tagRow struct {
	ID         uuid.UUID `db:"id"`
	DocumentID uuid.UUID `db:"document_id"`
	Key        string    `db:"key"`
	Value      string    `db:"value"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := postgres.NewDB(&cfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	checked, updated, err := backfillTags(ctx, db)
	if err != nil {
		return err
	}

	log.Printf("Backfill complete: %d tags checked, %d updated", checked, updated)
	return nil
}

func backfillTags(ctx context.Context, db *sqlx.DB) (checked, updated int, retErr error) {
	offset := 0

	for {
		var tags []tagRow
		if retErr = db.SelectContext(ctx, &tags,
			`SELECT id, document_id, key, value FROM document_tags
			 WHERE key IN ('seller_name', 'buyer_name')
			 ORDER BY id
			 LIMIT $1 OFFSET $2`, batchSize, offset); retErr != nil {
			retErr = fmt.Errorf("querying tags at offset %d: %w", offset, retErr)
			return
		}
		if len(tags) == 0 {
			break
		}

		for i := range tags {
			tag := &tags[i]
			normalized := normalize.CompanyName(tag.Value)
			checked++

			if normalized != tag.Value {
				if _, retErr = db.ExecContext(ctx,
					`UPDATE document_tags SET value = $1 WHERE id = $2`,
					normalized, tag.ID); retErr != nil {
					log.Printf("WARN: failed to update tag %s (doc %s): %v", tag.ID, tag.DocumentID, retErr)
					retErr = nil
					continue
				}
				log.Printf("Updated tag %s: %q → %q (doc %s)", tag.Key, tag.Value, normalized, tag.DocumentID)
				updated++
			}
		}

		offset += len(tags)

		if checked%5000 == 0 {
			log.Printf("Progress: %d tags checked, %d updated", checked, updated)
		}
	}

	return
}
