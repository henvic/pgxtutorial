package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/telemetry"
)

// NewHTTPServerAPI creates an HTTPServer for the API.
func NewHTTPServerAPI(i *inventory.Service, tel telemetry.Provider) http.Handler {
	s := &HTTPServerAPI{
		inventory: i,
		tel:       tel,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/product/", s.handleGetProduct)
	mux.HandleFunc("/review/", s.handleGetProductReview)
	return mux
}

// HTTPServerAPI exposes inventory.Service via HTTP.
type HTTPServerAPI struct {
	inventory *inventory.Service
	tel       telemetry.Provider
}

func (s *HTTPServerAPI) handleGetProduct(w http.ResponseWriter, r *http.Request) {
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
		s.tel.Logger().Error("internal server error getting product",
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
			s.tel.Logger().Info("cannot json encode product request",
				slog.Any("error", err),
			)
		}
	}
}

func (s *HTTPServerAPI) handleGetProductReview(w http.ResponseWriter, r *http.Request) {
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
		s.tel.Logger().Error("internal server error getting review",
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
			s.tel.Logger().Info("cannot json encode review request",
				slog.Any("error", err),
			)
		}
	}
}
