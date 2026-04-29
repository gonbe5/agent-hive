package feishu

import (
	"context"
	"strconv"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/chef-guo/agents-hive/internal/errs"
)

const defaultGapFetchPageSize = 50

type GapFetchContainerIDType string

const (
	GapFetchContainerIDTypeChat GapFetchContainerIDType = "chat"
)

type GapFetchSortType string

const (
	GapFetchSortByCreateTimeAsc GapFetchSortType = "ByCreateTimeAsc"
)

type GapFetchWindow struct {
	StartTime time.Time
	EndTime   time.Time
}

func (w GapFetchWindow) Validate() error {
	if w.StartTime.IsZero() {
		return errs.New(errs.CodeInvalidArgument, "gap fetch 缺少 start_time")
	}
	if w.EndTime.IsZero() {
		return errs.New(errs.CodeInvalidArgument, "gap fetch 缺少 end_time")
	}
	if w.EndTime.Before(w.StartTime) {
		return errs.New(errs.CodeInvalidArgument, "gap fetch end_time 不能早于 start_time")
	}
	return nil
}

type GapFetchRequest struct {
	ContainerIDType GapFetchContainerIDType
	ContainerID     string
	Window          GapFetchWindow
	PageSize        int
	PageToken       string
	SortType        GapFetchSortType
}

func (r GapFetchRequest) Validate() error {
	if r.ContainerIDType != GapFetchContainerIDTypeChat {
		return errs.New(errs.CodeInvalidArgument, "gap fetch 仅支持 container_id_type=chat")
	}
	if r.ContainerID == "" {
		return errs.New(errs.CodeInvalidArgument, "gap fetch 缺少 container_id")
	}
	if err := r.Window.Validate(); err != nil {
		return err
	}
	if r.PageSize < 0 {
		return errs.New(errs.CodeInvalidArgument, "gap fetch page_size 不能为负数")
	}
	return nil
}

func (r GapFetchRequest) normalizedPageSize() int {
	if r.PageSize <= 0 {
		return defaultGapFetchPageSize
	}
	return r.PageSize
}

func (r GapFetchRequest) normalizedSortType() GapFetchSortType {
	if r.SortType == "" {
		return GapFetchSortByCreateTimeAsc
	}
	return r.SortType
}

type GapFetchRequestParams struct {
	ContainerIDType string
	ContainerID     string
	StartTime       string
	EndTime         string
	PageSize        int
	PageToken       string
	SortType        string
}

func (r GapFetchRequest) Params() (GapFetchRequestParams, error) {
	if err := r.Validate(); err != nil {
		return GapFetchRequestParams{}, err
	}
	return GapFetchRequestParams{
		ContainerIDType: string(r.ContainerIDType),
		ContainerID:     r.ContainerID,
		StartTime:       strconv.FormatInt(r.Window.StartTime.Unix(), 10),
		EndTime:         strconv.FormatInt(r.Window.EndTime.Unix(), 10),
		PageSize:        r.normalizedPageSize(),
		PageToken:       r.PageToken,
		SortType:        string(r.normalizedSortType()),
	}, nil
}

func (r GapFetchRequest) BuildListMessageReq() (*larkim.ListMessageReq, error) {
	params, err := r.Params()
	if err != nil {
		return nil, err
	}

	req := larkim.NewListMessageReqBuilder().
		ContainerIdType(params.ContainerIDType).
		ContainerId(params.ContainerID).
		StartTime(params.StartTime).
		EndTime(params.EndTime).
		SortType(params.SortType).
		PageSize(params.PageSize)

	if params.PageToken != "" {
		req = req.PageToken(params.PageToken)
	}

	return req.Build(), nil
}

type GapFetchMessage struct {
	MessageID string
	Raw       *larkim.Message
}

type GapFetchPageResponse struct {
	Items         []GapFetchMessage
	HasMore       bool
	NextPageToken string
}

type gapFetchPageLister interface {
	ListGapMessages(ctx context.Context, req GapFetchRequest) (GapFetchPageResponse, error)
}

type GapFetchWalker struct {
	lister gapFetchPageLister
}

func NewGapFetchWalker(lister gapFetchPageLister) *GapFetchWalker {
	return &GapFetchWalker{lister: lister}
}

func (w *GapFetchWalker) FetchAll(ctx context.Context, req GapFetchRequest) ([]GapFetchPageResponse, error) {
	if w == nil || w.lister == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "gap fetch walker 未配置 lister")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	current := req
	var pages []GapFetchPageResponse
	for {
		page, err := w.lister.ListGapMessages(ctx, current)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
		if !page.HasMore {
			return pages, nil
		}
		if page.NextPageToken == "" {
			return nil, errs.New(errs.CodeChannelSendFailed, "gap fetch 返回 has_more=true 但缺少 page_token")
		}
		current.PageToken = page.NextPageToken
	}
}
