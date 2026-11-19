// Package worker drains the durable ingest queue.
package worker

import (
	"context"
	"log"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

const (
	batchSize  = 16
	maxRetries = 5
)

// Run processes queued ingest jobs until ctx is cancelled.
func Run(ctx context.Context, st store.Store, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		runOnce(ctx, st)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func runOnce(ctx context.Context, st store.Store) {
	for ctx.Err() == nil {
		jobs, err := st.ClaimIngestJobs(ctx, batchSize)
		if err != nil {
			log.Printf("ingest worker: claim: %v", err)
			return
		}
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			if err := st.ProcessIngestJob(ctx, job); err != nil {
				attempts := job.Attempts + 1
				log.Printf("ingest worker: job %d attempt %d failed: %v", job.ID, attempts, err)
				if ferr := st.FailIngestJob(ctx, job.ID, attempts, err.Error()); ferr != nil {
					log.Printf("ingest worker: job %d could not record failure: %v", job.ID, ferr)
				}
				if attempts >= maxRetries {
					log.Printf("ingest worker: job %d permanently failed after %d attempts", job.ID, attempts)
				}
				continue
			}
			if err := st.CompleteIngestJob(ctx, job.ID); err != nil {
				log.Printf("ingest worker: job %d could not mark done: %v", job.ID, err)
			}
		}
	}
}
