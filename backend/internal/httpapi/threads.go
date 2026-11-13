package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/jobs"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

const (
	maxMessageRunes = 4000
	defaultMessages = 50
	maxMessages     = 200
)

type threadService struct {
	buddyv1.UnimplementedThreadServiceServer
	deps Deps
}

var threadTypes = map[buddyv1.ThreadType]domain.ThreadType{
	buddyv1.ThreadType_THREAD_TYPE_GENERAL: domain.ThreadGeneral,
	buddyv1.ThreadType_THREAD_TYPE_PLAN:    domain.ThreadPlan,
	buddyv1.ThreadType_THREAD_TYPE_REQUEST: domain.ThreadRequest,
	buddyv1.ThreadType_THREAD_TYPE_TASK:    domain.ThreadTask,
}

func (s *threadService) ListThreads(ctx context.Context, _ *buddyv1.ListThreadsRequest) (*buddyv1.ListThreadsResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.deps.Threads.Threads(ctx, p.UserID)
	if err != nil {
		return nil, storeError(err)
	}
	resp := &buddyv1.ListThreadsResponse{Threads: []*buddyv1.Thread{}}
	for _, t := range list {
		pt, err := threadProto(t)
		if err != nil {
			return nil, status.Error(codes.Internal, "thread data")
		}
		resp.Threads = append(resp.Threads, pt)
	}
	return resp, nil
}

func (s *threadService) CreateThread(ctx context.Context, req *buddyv1.CreateThreadRequest) (*buddyv1.CreateThreadResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	typ, ok := threadTypes[req.GetType()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
	pin, err := pinFrom(req.GetPin())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if typ == domain.ThreadTask && (pin.Task == nil || pin.Task.Key == "") {
		return nil, status.Error(codes.InvalidArgument, "task thread needs pin.task.key")
	}

	// The plan thread lives on the plan tab: one per user.
	if typ == domain.ThreadPlan {
		existing, err := s.deps.Threads.Threads(ctx, p.UserID)
		if err != nil {
			return nil, storeError(err)
		}
		for _, t := range existing {
			if t.Type == domain.ThreadPlan {
				return threadResponse(t)
			}
		}
	}
	t, err := s.deps.Threads.CreateThread(ctx, p.UserID, typ, req.GetTitle(), pin)
	if err != nil {
		return nil, storeError(err)
	}
	return threadResponse(t)
}

func (s *threadService) GetThread(ctx context.Context, req *buddyv1.GetThreadRequest) (*buddyv1.GetThreadResponse, error) {
	t, err := s.ownThread(ctx, req.GetThreadId())
	if err != nil {
		return nil, err
	}
	pt, err := threadProto(t)
	if err != nil {
		return nil, status.Error(codes.Internal, "thread data")
	}
	return &buddyv1.GetThreadResponse{Thread: pt}, nil
}

func (s *threadService) ListMessages(ctx context.Context, req *buddyv1.ListMessagesRequest) (*buddyv1.ListMessagesResponse, error) {
	t, err := s.ownThread(ctx, req.GetThreadId())
	if err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultMessages
	}
	limit = min(limit, maxMessages)
	msgs, err := s.deps.Threads.Messages(ctx, t.ID, limit)
	if err != nil {
		return nil, storeError(err)
	}
	resp := &buddyv1.ListMessagesResponse{Messages: []*buddyv1.Message{}}
	for _, m := range msgs {
		pm, err := messageProto(m)
		if err != nil {
			return nil, status.Error(codes.Internal, "message data")
		}
		resp.Messages = append(resp.Messages, pm)
	}
	return resp, nil
}

func (s *threadService) SendMessage(ctx context.Context, req *buddyv1.SendMessageRequest) (*buddyv1.SendMessageResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	if content == "" || utf8.RuneCountInString(content) > maxMessageRunes {
		return nil, status.Errorf(codes.InvalidArgument, "content must be 1..%d characters", maxMessageRunes)
	}
	t, err := s.ownThread(ctx, req.GetThreadId())
	if err != nil {
		return nil, err
	}
	msg, err := s.deps.Threads.AddMessage(ctx, domain.Message{ThreadID: t.ID, Role: domain.MessageUser, Content: content})
	if err != nil {
		return nil, storeError(err)
	}
	mode := "brief"
	if req.GetMode() == buddyv1.AnswerMode_ANSWER_MODE_DETAILED {
		mode = "detailed"
	}
	jobID, err := s.deps.Jobs.EnqueueChatTurn(ctx, jobs.ChatTurn{ThreadID: t.ID, UserID: t.UserID, MessageID: msg.ID, Mode: mode})
	if err != nil {
		return nil, status.Error(codes.Unavailable, "could not queue the answer, try again")
	}
	return &buddyv1.SendMessageResponse{MessageId: msg.ID, JobId: jobID}, nil
}

// ownThread returns NotFound for missing threads and for threads of other users alike.
func (s *threadService) ownThread(ctx context.Context, id string) (domain.Thread, error) {
	p, err := principal(ctx)
	if err != nil {
		return domain.Thread{}, err
	}
	t, err := s.deps.Threads.Thread(ctx, p.UserID, id)
	if err != nil {
		return domain.Thread{}, storeError(err)
	}
	return t, nil
}

// maxFeedbackNote keeps free-text feedback short.
const maxFeedbackNote = 1000

func (s *threadService) RateMessage(ctx context.Context, req *buddyv1.RateMessageRequest) (*buddyv1.RateMessageResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	var value string
	switch req.GetValue() {
	case buddyv1.Rating_RATING_UP:
		value = "up"
	case buddyv1.Rating_RATING_DOWN:
		value = "down"
	default:
		return nil, status.Error(codes.InvalidArgument, "value must be RATING_UP or RATING_DOWN")
	}
	if len([]rune(req.GetNote())) > maxFeedbackNote {
		return nil, status.Error(codes.InvalidArgument, "note is too long")
	}
	if err := s.deps.Threads.RateMessage(ctx, p.UserID, req.GetMessageId(), value, req.GetNote()); err != nil {
		return nil, storeError(err)
	}
	return &buddyv1.RateMessageResponse{}, nil
}

func principal(ctx context.Context) (domain.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return domain.Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return p, nil
}

func storeError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return status.Error(codes.NotFound, "not found")
	}
	return status.Error(codes.Unavailable, "storage unavailable")
}

// pinFrom accepts only the known pin shapes, so arbitrary JSON never lands in the database.
func pinFrom(s *structpb.Struct) (domain.ThreadPin, error) {
	var pin domain.ThreadPin
	if s == nil {
		return pin, nil
	}
	raw, err := s.MarshalJSON()
	if err != nil {
		return pin, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pin); err != nil {
		return pin, errors.New("pin: " + err.Error())
	}
	return pin, nil
}

func threadResponse(t domain.Thread) (*buddyv1.CreateThreadResponse, error) {
	pt, err := threadProto(t)
	if err != nil {
		return nil, status.Error(codes.Internal, "thread data")
	}
	return &buddyv1.CreateThreadResponse{Thread: pt}, nil
}

func threadProto(t domain.Thread) (*buddyv1.Thread, error) {
	pin, err := toStruct(t.Pin)
	if err != nil {
		return nil, err
	}
	out := &buddyv1.Thread{Id: t.ID, Title: t.Title, Pin: pin, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339)}
	for pt, dt := range threadTypes {
		if dt == t.Type {
			out.Type = pt
		}
	}
	return out, nil
}

func messageProto(m domain.Message) (*buddyv1.Message, error) {
	out := &buddyv1.Message{
		Id: m.ID, ThreadId: m.ThreadID, Text: m.Content, Blocks: []*structpb.Struct{},
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
	}
	switch m.Role {
	case domain.MessageUser:
		out.Role = buddyv1.MessageRole_MESSAGE_ROLE_USER
	case domain.MessageAssistant:
		out.Role = buddyv1.MessageRole_MESSAGE_ROLE_ASSISTANT
	}
	if len(m.Blocks) > 0 {
		var blocks []map[string]any
		if err := json.Unmarshal(m.Blocks, &blocks); err != nil {
			return nil, err
		}
		for _, b := range blocks {
			st, err := structpb.NewStruct(b)
			if err != nil {
				return nil, err
			}
			out.Blocks = append(out.Blocks, st)
		}
	}
	return out, nil
}

func toStruct(v any) (*structpb.Struct, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}
