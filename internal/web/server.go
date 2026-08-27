package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"radio-observation-release-gate/internal/application"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	app    *application.Service
	logger *slog.Logger
	mux    *http.ServeMux
}

func New(app *application.Service, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{app: app, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s.logging(s.securityHeaders(s.mux))
}

func (s *Server) routes() {
	static, _ := fs.Sub(assets, "assets")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /", s.WorkbenchHandler)
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("GET /api/batches", s.ListBatchesHandler)
	s.mux.HandleFunc("POST /api/batches", s.CreateBatchHandler)
	s.mux.HandleFunc("GET /api/batches/{batchID}", s.GetBatchHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/freeze", s.FreezeBaselineHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/segments", s.RegisterSegmentHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/segments/bulk", s.RegisterSegmentsHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/quality", s.AssessSegmentHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/quality/bulk", s.AssessBatchHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/replacement-preview", s.PreviewReplacementHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/quarantine", s.QuarantineHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/reviews", s.GenerateReviewHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/review-decisions", s.DecideReviewHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/review-assignments", s.AssignReviewHandler)
	s.mux.HandleFunc("POST /api/batches/{batchID}/seal", s.SealHandler)
	s.mux.HandleFunc("GET /api/batches/{batchID}/timeline", s.TimelineHandler)
	s.mux.HandleFunc("GET /api/batches/{batchID}/manifest/verify", s.VerifyManifestHandler)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
