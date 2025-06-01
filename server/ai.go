package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/expki/go-vectorsearch/logger"
	"github.com/expki/go-vectorsearch/server"
)

type AIRequest struct {
	Query      string `json:"query"`
	DocumentID uint64 `json:"id"`
}

func (s *Server) AIHttp(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	txid := index.Add(1)
	logger.Sugar().Debugf("%d AI request started", txid)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Ensure the ResponseWriter supports streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Ensure the request method is POST or GET
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		logger.Sugar().Debugf("%d request method denied: %s", txid, r.Method)
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, `Invalid request method`)
		return
	}

	// Read the request body
	logger.Sugar().Debugf("%d reading request body", txid)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Sugar().Debugf("%d request body invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `Invalid request body`)
		return
	}
	defer r.Body.Close()

	// Parse the JSON request body into the RequestBody struct
	logger.Sugar().Debugf("%d unmarshing request body", txid)
	var req AIRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Sugar().Debugf("%d request invalid: %v", txid, err)
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `Invalid request`)
		return
	}

	// Handle the chat request
	logger.Sugar().Debugf("%d starting chat", txid)
	resStream, err := s.vs.Chat(r.Context(), server.ChatRequest{
		Prefix:      req.Query,
		Text:        "Produce a one paragraph summary on the information and do not ask any followup questions",
		DocumentIDs: []uint64{req.DocumentID},
	})
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		// request canceled
		logger.Sugar().Warnf("%d chat request canceled after %s", txid, time.Since(start).String())
		return
	} else {
		http.Error(w, "Failed to initialize AI", http.StatusInternalServerError)
		return
	}
	defer resStream.Close()
	charReader := bufio.NewReader(resStream)

	// Stream the response
	logger.Sugar().Debugf("%d starting stream", txid)
	for {
		select {
		case <-r.Context().Done():
			logger.Sugar().Warnf("%d chat stream canceled after %s", txid, time.Since(start).String())
			w.WriteHeader(499)
			io.WriteString(w, "Request AI canceled")
			return
		default:
		}
		char, _, err := charReader.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			http.Error(w, "Error reading stream", http.StatusInternalServerError)
			return
		}

		_, writeErr := io.WriteString(w, string(char))
		if writeErr != nil {
			logger.Sugar().Warnf("%d chat write failed: %v (%s)", txid, writeErr, time.Since(start).String())
			return
		}

		flusher.Flush()
	}

	logger.Sugar().Infof("%d AI request suceeded (%dms)", txid, time.Since(start).Milliseconds())
}
