package wanthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/internal/stores"
	"wantbuild.io/want/src/wantjob"
)

var _ wantjob.System = &Client{}

type (
	Idx    = wantjob.Idx
	Result = wantjob.Result
	Job    = wantjob.Job
	Task   = wantjob.Task
)

type Client struct {
	hc  *http.Client
	url string
}

func NewClient(hc *http.Client, u string) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{hc: hc, url: u}
}

func (c *Client) Await(ctx context.Context, idx Idx) error {
	var resp AwaitJobResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.Await", AwaitJobReq{Idx: idx}, &resp); err != nil {
		return err
	}
	return nil
}

func (c *Client) Cancel(ctx context.Context, idx Idx) error {
	var resp CancelJobResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.Cancel", CancelJobReq{Idx: idx}, &resp); err != nil {
		return err
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, idx Idx) error {
	return c.doJSON(ctx, http.MethodPost, "/jobs.Delete", DeleteJobReq{Idx: idx}, &DeleteJobResp{})
}

func (c *Client) Inspect(ctx context.Context, idx Idx) (*Job, error) {
	var resp InspectJobResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.Inspect", InspectJobReq{Idx: idx}, &resp); err != nil {
		return nil, err
	}
	return &resp.Job, nil
}

func (c *Client) List(ctx context.Context) ([]Idx, error) {
	var resp ListJobsResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.List", ListJobsReq{}, &resp); err != nil {
		return nil, err
	}
	return resp.Idxs, nil
}

func (c *Client) Spawn(ctx context.Context, src cadata.Getter, task Task) (Idx, error) {
	var resp SpawnResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.Spawn", SpawnReq{Task: task}, &resp); err != nil {
		return 0, err
	}
	return resp.Idx, nil
}

func (c *Client) ViewResult(ctx context.Context, idx Idx) (*Result, cadata.Getter, error) {
	req := ViewResultReq{Idx: idx}
	var resp ViewResultResp
	if err := c.doJSON(ctx, http.MethodPost, "/jobs.ViewResult", req, &resp); err != nil {
		return nil, nil, err
	}
	return &resp.Result, c.Store(resp.StoreID), nil
}

func (c *Client) doJSON(ctx context.Context, method, p string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	status, respBody, err := c.do(ctx, method, p, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("status %d", status)
	}
	return json.Unmarshal(respBody, resp)
}

func (c *Client) do(ctx context.Context, method, p string, reqBody []byte) (int, []byte, error) {
	u := strings.TrimRight(c.url, "/") + "/" + strings.TrimLeft(p, "/")
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func (c *Client) Store(sid int) *Store {
	return &Store{hc: c, sid: sid}
}

func (c *Client) GetTask(ctx context.Context) (*Task, error) {
	var resp GetTaskResp
	if err := c.doJSON(ctx, http.MethodPost, "/task", GetTaskReq{}, &resp); err != nil {
		return nil, err
	}
	return resp.Task, nil
}

func (c *Client) SetResult(ctx context.Context, result Result) error {
	var resp SetResultResp
	if err := c.doJSON(ctx, http.MethodPost, "/result", SetResultReq{Result: result}, &resp); err != nil {
		return err
	}
	return nil
}

var _ cadata.Store = &Store{}

type Store struct {
	hc  *Client
	sid int
}

func (s *Store) Post(ctx context.Context, data []byte) (cadata.ID, error) {
	req := PostReq{Data: data}
	var resp PostResp
	if err := s.hc.doJSON(ctx, http.MethodPost, fmt.Sprintf("/stores/%d.Post", s.sid), req, &resp); err != nil {
		return cadata.ID{}, err
	}
	return resp.CID, nil
}

func (s *Store) Get(ctx context.Context, id cadata.ID, buf []byte) (int, error) {
	reqData, err := json.Marshal(GetReq{CID: id})
	if err != nil {
		return 0, err
	}
	status, respBody, err := s.hc.do(ctx, http.MethodPost, fmt.Sprintf("/stores/%d.Get", s.sid), reqData)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("status %d", status)
	}
	return copy(buf, respBody), nil
}

func (s *Store) Delete(ctx context.Context, id cadata.ID) error {
	req := DeleteBlobReq{CID: id}
	var resp DeleteBlobResp
	return s.hc.doJSON(ctx, http.MethodPost, fmt.Sprintf("/stores/%d.Delete", s.sid), req, &resp)
}

func (s *Store) Exists(ctx context.Context, id cadata.ID) (bool, error) {
	req := BlobExistsReq{CIDs: []cadata.ID{id}}
	var resp BlobExistsResp
	if err := s.hc.doJSON(ctx, http.MethodPost, fmt.Sprintf("/stores/%d.Exists", s.sid), req, &resp); err != nil {
		return false, err
	}
	return resp.Exists[0], nil
}

func (s *Store) List(ctx context.Context, span cadata.Span, ids []cadata.ID) (int, error) {
	var req ListBlobsReq
	if gteq, ok := span.LowerBound(); ok {
		req.Gteq = &gteq
	}
	if lt, ok := span.UpperBound(); ok {
		req.Lt = &lt
	}
	var resp ListBlobsResp
	if err := s.hc.doJSON(ctx, http.MethodPost, fmt.Sprintf("/stores/%d.List", s.sid), req, &resp); err != nil {
		return 0, err
	}
	return 0, nil
}

func (s *Store) MaxSize() int {
	return stores.MaxBlobSize
}

func (s *Store) Hash(x []byte) cadata.ID {
	return stores.Hash(x)
}
