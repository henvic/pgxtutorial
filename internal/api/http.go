package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/henvic/pgxtutorial/internal/inventory"
	"golang.org/x/exp/slog"
)

// NewHTTPServer creates an HTTPServer for the API.
func NewHTTPServer(i *inventory.Service, logger *slog.Logger) http.Handler {
	s := &HTTPServer{
		inventory: i,
		log:       logger,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("/product/", s.handleGetProduct)
	s.mux.HandleFunc("/review/", s.handleGetProductReview)
	return s.mux
}

// HTTPServer exposes inventory.Service via HTTP.
type HTTPServer struct {
	inventory *inventory.Service
	log       *slog.Logger
	mux       *http.ServeMux
}

func (s *HTTPServer) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/product/"):]
	if id == "" || strings.ContainsRune(id, '/') {
		http.NotFound(w, r)
		return
	}
	review, err := s.inventory.GetProduct(r.Context(), id)
	switch {
	case err == context.Canceled, err == context.DeadlineExceeded:
		return
	case err != nil:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		s.log.Error("internal server error getting product",
			slog.Any("code", http.StatusInternalServerError),
			slog.Any("error", err),
		)
	case review == nil:
		http.Error(w, "Product not found", http.StatusNotFound)
	default:
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "\t")
		if err := enc.Encode(review); err != nil {
			s.log.Info("cannot json encode product request",
				slog.Any("error", err),
			)
		}
	}
}

func (s *HTTPServer) handleGetProductReview(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/review/"):]
	if id == "" || strings.ContainsRune(id, '/') {
		http.NotFound(w, r)
		return
	}
	review, err := s.inventory.GetProductReview(r.Context(), id)
	switch {
	case err == context.Canceled, err == context.DeadlineExceeded:
		return
	case err != nil:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		s.log.Error("internal server error getting review",
			slog.Any("code", http.StatusInternalServerError),
			slog.Any("error", err),
		)
	case review == nil:
		http.Error(w, "Review not found", http.StatusNotFound)
	default:
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "\t")
		if err := enc.Encode(review); err != nil {
			s.log.Info("cannot json encode review request",
				slog.Any("error", err),
			)
		}
	}
}
