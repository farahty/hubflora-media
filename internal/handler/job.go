package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/queue"
	"github.com/farahty/hubflora-media/internal/storage"
)

// JobStatusResponse is the response for job status queries.
type JobStatusResponse struct {
	JobID        string `json:"jobId"`
	State        string `json:"state"`
	Progress     int    `json:"progress"`
	ProcessedOn  int64  `json:"processedOn,omitempty"`
	FinishedOn   int64  `json:"finishedOn,omitempty"`
	FailedReason string `json:"failedReason,omitempty"`
}

// JobStatus handles GET /api/v1/media/job/{jobId}.
// Returns the current state of an async variant generation job.
func JobStatus(inspector *asynq.Inspector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "jobId")
		if jobID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jobId is required"})
			return
		}

		if inspector == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "async processing not available"})
			return
		}

		// Try to find the task in various states
		info, err := inspector.GetTaskInfo("default", jobID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}

		resp := JobStatusResponse{
			JobID: jobID,
			State: info.State.String(),
		}

		if info.CompletedAt.Unix() > 0 {
			resp.FinishedOn = info.CompletedAt.UnixMilli()
		}

		if info.LastFailedAt.Unix() > 0 {
			resp.FailedReason = info.LastErr
		}

		// Map asynq states to progress
		switch info.State {
		case asynq.TaskStateCompleted:
			resp.Progress = 100
		case asynq.TaskStateActive:
			resp.Progress = 50
		case asynq.TaskStatePending:
			resp.Progress = 0
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// VariantRegenerate handles POST /api/v1/media/variants.
// Triggers variant regeneration for an existing file.
func VariantRegenerate(cfg *config.Config, asynqClient *asynq.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ObjectKey  string   `json:"objectKey"`
			BucketName string   `json:"bucketName"`
			Variants   []string `json:"variants"` // reserved for future selective regeneration
		}

		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		if req.ObjectKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "objectKey is required"})
			return
		}

		if asynqClient == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "async processing not available"})
			return
		}

		bucket := req.BucketName
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		folderPath := storage.ExtractFolderPath(req.ObjectKey)

		task, err := queue.NewVariantTask("regen", bucket, folderPath, req.ObjectKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create task"})
			return
		}

		info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue task"})
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"jobId": info.ID})
	}
}