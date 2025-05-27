package wanthttp

import (
	"go.brendoncarroll.net/state/cadata"

	"wantbuild.io/want/src/wantjob"
)

type SpawnReq struct {
	Task wantjob.Task `json:"task"`
}

type SpawnResp struct {
	Idx wantjob.Idx `json:"idx"`
}

type InspectJobReq struct {
	Idx wantjob.Idx `json:"idx"`
}

type InspectJobResp struct {
	Job wantjob.Job `json:"job"`
}

type AwaitJobReq struct {
	Idx wantjob.Idx `json:"idx"`
}

type AwaitJobResp struct{}

type CancelJobReq struct {
	Idx wantjob.Idx `json:"idx"`
}

type CancelJobResp struct{}

type ViewResultReq struct {
	Idx wantjob.Idx `json:"idx"`
}

type ViewResultResp struct {
	Result  wantjob.Result `json:"result"`
	StoreID int            `json:"store_id"`
}

type DeleteJobReq struct {
	Idx wantjob.Idx `json:"idx"`
}

type DeleteJobResp struct{}

type ListJobsReq struct{}

type ListJobsResp struct {
	Idxs []wantjob.Idx `json:"idxs"`
}

type PostReq struct {
	Data []byte `json:"data"`
}

type PostResp struct {
	CID cadata.ID `json:"cid"`
}

type GetReq struct {
	CID cadata.ID `json:"cid"`
}

type DeleteBlobReq struct {
	CID cadata.ID `json:"cid"`
}

type DeleteBlobResp struct{}

type BlobExistsReq struct {
	CIDs []cadata.ID `json:"cid"`
}

type BlobExistsResp struct {
	Exists []bool `json:"exists"`
}

type ListBlobsReq struct {
	Gteq *cadata.ID `json:"gteq,omitempty"`
	Lt   *cadata.ID `json:"lt,omitempty"`
}

type ListBlobsResp struct {
	CIDs []cadata.ID `json:"cids"`
}
