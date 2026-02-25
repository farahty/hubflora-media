package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health returns a health check handler that also verifies DB connectivity.
func Health(dbPool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		if err := dbPool.Ping(r.Context()); err != nil {
			status = "db_unhealthy"
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}
