package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"ashen/metadata-worker/internal/extract"
	"ashen/metadata-worker/internal/jobs"
	"ashen/metadata-worker/internal/normalize"
	"ashen/metadata-worker/internal/store"
)

type Processor struct {
	store     *store.Store
	extractor *extract.Extractor
	client    *asynq.Client
}

func New(st *store.Store, ex *extract.Extractor, client *asynq.Client) *Processor {
	return &Processor{store: st, extractor: ex, client: client}
}

// HandleExtract downloads the original, runs exiftool/ffprobe, stores raw, then
// chains NORMALIZE.
func (p *Processor) HandleExtract(ctx context.Context, t *asynq.Task) error {
	var payload jobs.ExtractPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("bad payload: %w: %w", err, asynq.SkipRetry)
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeExtract, jobs.WorkerVersion, "processing", "")

	raw, err := p.extractor.Run(ctx, payload.Bucket, payload.StorageKey, payload.MediaType)
	if err != nil {
		_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeExtract, jobs.WorkerVersion, "failed_retryable", err.Error())
		return err // Asynq retries with backoff
	}
	if err := p.store.SaveRaw(ctx, payload.AssetID, raw, "exiftool", jobs.WorkerVersion); err != nil {
		return err
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeExtract, jobs.WorkerVersion, "completed", "")

	return p.enqueue(ctx, jobs.TypeNormalize, payload.AssetID)
}

// HandleNormalize reads raw, writes technical fields, then chains INDEX.
func (p *Processor) HandleNormalize(ctx context.Context, t *asynq.Task) error {
	var payload jobs.AssetPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("bad payload: %w: %w", err, asynq.SkipRetry)
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeNormalize, jobs.WorkerVersion, "processing", "")

	raw, err := p.store.RawMetadata(ctx, payload.AssetID)
	if err != nil {
		_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeNormalize, jobs.WorkerVersion, "failed_retryable", err.Error())
		return err
	}
	tech := normalize.Normalize(raw)
	if err := p.store.SaveTechnical(ctx, payload.AssetID, tech); err != nil {
		return err
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeNormalize, jobs.WorkerVersion, "completed", "")

	return p.enqueue(ctx, jobs.TypeIndex, payload.AssetID)
}

// HandleIndex builds the v1 search document (filename + date + camera).
func (p *Processor) HandleIndex(ctx context.Context, t *asynq.Task) error {
	var payload jobs.AssetPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("bad payload: %w: %w", err, asynq.SkipRetry)
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeIndex, jobs.WorkerVersion, "processing", "")

	filename, camera, capturedAt, err := p.store.TechnicalText(ctx, payload.AssetID)
	if err != nil {
		_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeIndex, jobs.WorkerVersion, "failed_retryable", err.Error())
		return err
	}
	parts := []string{filename, camera}
	if capturedAt != nil {
		parts = append(parts, capturedAt.Format("2006 January 2"))
	}
	text := strings.TrimSpace(strings.Join(nonEmpty(parts), " "))

	if err := p.store.SaveSearchDoc(ctx, payload.AssetID, text, jobs.WorkerVersion); err != nil {
		return err
	}
	_ = p.store.MarkJob(ctx, payload.AssetID, jobs.TypeIndex, jobs.WorkerVersion, "completed", "")
	return nil
}

func (p *Processor) enqueue(ctx context.Context, taskType, assetID string) error {
	task, err := jobs.NewAssetTask(taskType, assetID)
	if err != nil {
		return err
	}
	_, err = p.client.EnqueueContext(ctx, task, asynq.MaxRetry(5), asynq.Timeout(2*time.Minute))
	return err
}

func nonEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
