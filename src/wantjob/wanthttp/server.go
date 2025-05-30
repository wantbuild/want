package wanthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"go.brendoncarroll.net/state/cadata"
	"go.brendoncarroll.net/stdctx/logctx"
	"go.uber.org/zap"
	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
)

type Server struct {
	sys wantjob.System

	wstore cadata.Store

	mu           sync.Mutex
	input        []byte
	inputStore   cadata.Getter
	resultStores map[StoreID]cadata.Getter
	result       *Result
}

func NewServer(sys wantjob.System) *Server {
	return &Server{sys: sys, resultStores: make(map[StoreID]cadata.Getter)}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "must use http POST", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/stores/") {
		s.handleStore(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/jobs") {
		s.handleJob(w, r)
		return
	}

	switch r.URL.Path {
	case "/input":
		s.mu.Lock()
		defer s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write(s.input)
	case "/result":
		handleRequest(w, r, func(ctx context.Context, req SetResultReq) (*SetResultResp, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.result = &req.Result
			return &SetResultResp{}, nil
		})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) GetResult() *Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

func (s *Server) SetInput(src cadata.Getter, input []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input = input
	s.inputStore = src
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/jobs.Spawn":
		handleRequest(w, r, func(ctx context.Context, req SpawnReq) (*SpawnResp, error) {
			idx, err := s.sys.Spawn(ctx, s.wstore, req.Task)
			if err != nil {
				return nil, err
			}
			return &SpawnResp{Idx: idx}, nil
		})
	case "/jobs.Inspect":
		handleRequest(w, r, func(ctx context.Context, req InspectJobReq) (*InspectJobResp, error) {
			job, err := s.sys.Inspect(ctx, req.Idx)
			if err != nil {
				return nil, err
			}
			return &InspectJobResp{Job: *job}, nil
		})
	case "/jobs.Await":
		handleRequest(w, r, func(ctx context.Context, req AwaitJobReq) (*AwaitJobResp, error) {
			err := s.sys.Await(ctx, req.Idx)
			if err != nil {
				return nil, err
			}
			return &AwaitJobResp{}, nil
		})
	case "/jobs.Cancel":
		handleRequest(w, r, func(ctx context.Context, req CancelJobReq) (*CancelJobResp, error) {
			err := s.sys.Cancel(ctx, req.Idx)
			if err != nil {
				return nil, err
			}
			return &CancelJobResp{}, nil
		})
	case "/jobs.ViewResult":
		handleRequest(w, r, func(ctx context.Context, req ViewResultReq) (*ViewResultResp, error) {
			result, src, err := s.sys.ViewResult(ctx, req.Idx)
			if err != nil {
				return nil, err
			}
			storeID := StoreID(req.Idx)
			s.mu.Lock()
			s.resultStores[storeID] = src
			s.mu.Unlock()
			return &ViewResultResp{Result: *result, StoreID: storeID}, nil
		})
	case "/jobs.Delete":
		handleRequest(w, r, func(ctx context.Context, req DeleteJobReq) (*DeleteJobResp, error) {
			err := s.sys.Delete(ctx, req.Idx)
			if err != nil {
				return nil, err
			}
			storeID := StoreID(req.Idx)
			s.mu.Lock()
			delete(s.resultStores, storeID)
			s.mu.Unlock()
			return &DeleteJobResp{}, nil
		})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	var storeID StoreID
	var method string
	if _, err := fmt.Sscanf(r.URL.Path, "/stores/%d.%s", &storeID, &method); err != nil {
		http.Error(w, "could not parse path", http.StatusBadRequest)
		return
	}
	switch method {
	case "Post":
		handleRequest(w, r, func(ctx context.Context, req PostReq) (*PostResp, error) {
			cid, err := s.wstore.Post(ctx, req.Data)
			if err != nil {
				return nil, err
			}
			return &PostResp{CID: cid}, nil
		})
	case "Get":
		reqData, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req GetReq
		if err := json.Unmarshal(reqData, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		store := s.getStore(storeID)
		buf := make([]byte, store.MaxSize())
		n, err := store.Get(r.Context(), req.CID, buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(buf[:n])
	case "Delete":
		handleRequest(w, r, func(ctx context.Context, req DeleteBlobReq) (*DeleteBlobResp, error) {
			if err := s.wstore.Delete(ctx, req.CID); err != nil {
				return nil, err
			}
			return &DeleteBlobResp{}, nil
		})
	case "Exists":
		handleRequest(w, r, func(ctx context.Context, req BlobExistsReq) (*BlobExistsResp, error) {
			exists := make([]bool, len(req.CIDs))
			for i, cid := range req.CIDs {
				var err error
				exists[i], err = s.wstore.Exists(r.Context(), cid)
				if err != nil {
					return nil, err
				}
			}
			return &BlobExistsResp{Exists: exists}, nil
		})
	case "List":
		handleRequest(w, r, func(ctx context.Context, req ListBlobsReq) (*ListBlobsResp, error) {
			return nil, fmt.Errorf("not implemented")
		})
	default:
		http.Error(w, "invalid method "+method, http.StatusBadRequest)
	}
}

func (s *Server) getStore(storeID StoreID) cadata.Getter {
	if storeID == CurrentStore {
		return stores.Union{s.inputStore, s.wstore}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resultStores[storeID]
}

func handleRequest[Req, Resp any](w http.ResponseWriter, r *http.Request, fn func(context.Context, Req) (*Resp, error)) {
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := fn(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respData, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(respData); err != nil {
		logctx.Warn(r.Context(), "writing http response", zap.Error(err))
	}
}
