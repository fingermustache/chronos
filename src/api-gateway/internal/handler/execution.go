package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/fingermustache/chronos/api-gateway/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ExecutionHandler struct {
	service service.ExecutionService
	logger  *slog.Logger
}

func NewExecutionHandler(svc service.ExecutionService, logger *slog.Logger) *ExecutionHandler {
	return &ExecutionHandler{service: svc, logger: logger}
}

// GET /tasks/:id/executions
func (h *ExecutionHandler) List(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	cursor := r.URL.Query().Get("cursor")

	result, err := h.service.ListByTask(r.Context(), taskID, service.ListExecutionsRequest{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskNotFound) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Error())
			return
		}
		h.logger.Error("failed to list executions", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
