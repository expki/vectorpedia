package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/plugin/dbresolver"
)

type CountResponse struct {
	Embeddings int64 `json:"embeddings"`
	Documents  int64 `json:"documents"`
	Centroids  int64 `json:"centroids"`
}

func (s *Server) CountHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d Count request started", txid)
	w.Header().Set("Content-Type", "application/json")

	// Ensure the request method is POST or GET
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		logger.Sugar().Debugf("%d request method denied: %s", txid, r.Method)
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, `Invalid request method`)
		return
	}

	resAny, err, _ := s.singleCount.Do("count", func() (any, error) {
		embeddings, documents, centroids, err := s.Count()
		return CountResponse{
			Embeddings: embeddings,
			Documents:  documents,
			Centroids:  centroids,
		}, err
	})
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		w.WriteHeader(499)
		io.WriteString(w, `{"error":"Request count canceled"}`)
		return
	} else {
		logger.Sugar().Errorf("%d count request failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Request count failed"}`)
		return
	}
	res := resAny.(CountResponse)

	// Create response bytes
	resBytes, err := json.Marshal(res)
	if err != nil {
		logger.Sugar().Errorf("%d count response marshal failed: %v", txid, err)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"Response failed"}`)
		return
	}

	// Set the response headers and write the JSON response
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	logger.Sugar().Infof("%d count request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}

func (s *Server) Count() (embeddings, documents, centroids int64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Handle the centroids request
	centroids, err = func() (centroids int64, err error) {
		if s.centroidCountCache.Load() {
			logger.Sugar().Debug("cache count centroids: enabled")
			return s.lastCentroidCount.Load(), nil
		}
		logger.Sugar().Debug("cache count centroids: disabled")
		err = s.db.DB.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Centroid{}).Count(&centroids).Error
		if err == nil {
			s.lastCentroidCount.Store(centroids)
		}
		return centroids, err
	}()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return
	} else {
		return embeddings, documents, centroids, errors.Join(errors.New("count centroids exception"), err)
	}

	// Handle the documents request
	documents, err = func() (documents int64, err error) {
		if s.documentCountCache.Load() {
			logger.Sugar().Debug("cache count documents: enabled")
			return s.lastDocumentCount.Load(), nil
		}
		logger.Sugar().Debug("cache count documents: disabled")
		err = s.db.DB.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Document{}).Count(&documents).Error
		if err == nil {
			s.lastDocumentCount.Store(documents)
		}
		return documents, err
	}()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return embeddings, documents, centroids, err
	} else {
		return embeddings, documents, centroids, errors.Join(errors.New("count documents exception"), err)
	}

	// Handle the embeddings request
	embeddings, err = func() (embeddings int64, err error) {
		if s.documentCountCache.Load() {
			logger.Sugar().Debug("cache count embeddings: enabled")
			return s.lastEmbeddingCount.Load(), nil
		}
		logger.Sugar().Debug("cache count embeddings: disabled")
		err = s.db.DB.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Embedding{}).Count(&embeddings).Error
		if err == nil {
			s.lastEmbeddingCount.Store(embeddings)
		}
		return embeddings, err
	}()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return embeddings, documents, centroids, err
	} else {
		return embeddings, documents, centroids, errors.Join(errors.New("count embeddings exception"), err)
	}

	return embeddings, documents, centroids, nil
}
