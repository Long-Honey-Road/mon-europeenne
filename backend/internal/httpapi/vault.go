package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	buddyv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/buddy/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const (
	maxVaultTitle   = 80
	maxVaultSummary = 1200
)

// Vault is the storage of the vault API; everything is scoped to the caller.
type Vault interface {
	CreateVaultItem(ctx context.Context, v domain.VaultItem) (domain.VaultItem, error)
	VaultItems(ctx context.Context, userID, tag string) ([]domain.VaultItem, error)
	VaultItem(ctx context.Context, userID, id string) (domain.VaultItem, error)
	UpdateVaultItem(ctx context.Context, userID, id string, p domain.VaultPatch) (domain.VaultItem, error)
	RestoreVaultVersion(ctx context.Context, userID, id string, version int) (domain.VaultItem, error)
	DeleteVaultItem(ctx context.Context, userID, id string) error
}

type vaultService struct {
	buddyv1.UnimplementedVaultServiceServer
	deps Deps
}

func (s *vaultService) ListVaultItems(ctx context.Context, req *buddyv1.ListVaultItemsRequest) (*buddyv1.ListVaultItemsResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.deps.Vault.VaultItems(ctx, p.UserID, strings.ToLower(strings.TrimSpace(req.GetTag())))
	if err != nil {
		return nil, storeError(err)
	}
	out := &buddyv1.ListVaultItemsResponse{Items: []*buddyv1.VaultItem{}}
	for _, v := range items {
		pv, err := vaultProto(v)
		if err != nil {
			return nil, status.Error(codes.Internal, "vault data")
		}
		out.Items = append(out.Items, pv)
	}
	return out, nil
}

func (s *vaultService) GetVaultItem(ctx context.Context, req *buddyv1.GetVaultItemRequest) (*buddyv1.GetVaultItemResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.deps.Vault.VaultItem(ctx, p.UserID, req.GetItemId())
	if err != nil {
		return nil, storeError(err)
	}
	pv, err := vaultProto(v)
	if err != nil {
		return nil, status.Error(codes.Internal, "vault data")
	}
	return &buddyv1.GetVaultItemResponse{Item: pv}, nil
}

func (s *vaultService) UpdateVaultItem(ctx context.Context, req *buddyv1.UpdateVaultItemRequest) (*buddyv1.UpdateVaultItemResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	var patch domain.VaultPatch
	if t := strings.TrimSpace(req.GetTitle()); t != "" {
		if len([]rune(t)) > maxVaultTitle {
			return nil, status.Errorf(codes.InvalidArgument, "title up to %d characters", maxVaultTitle)
		}
		patch.Title = &t
	}
	if sm := strings.TrimSpace(req.GetSummary()); sm != "" {
		if len([]rune(sm)) > maxVaultSummary {
			return nil, status.Errorf(codes.InvalidArgument, "summary up to %d characters", maxVaultSummary)
		}
		patch.Summary = &sm
	}
	if req.GetTags() != nil {
		patch.Tags = req.GetTags()
	}
	if patch.Title == nil && patch.Summary == nil && patch.Tags == nil {
		return nil, status.Error(codes.InvalidArgument, "nothing to change")
	}
	v, err := s.deps.Vault.UpdateVaultItem(ctx, p.UserID, req.GetItemId(), patch)
	if err != nil {
		return nil, storeError(err)
	}
	pv, err := vaultProto(v)
	if err != nil {
		return nil, status.Error(codes.Internal, "vault data")
	}
	return &buddyv1.UpdateVaultItemResponse{Item: pv}, nil
}

func (s *vaultService) RestoreVaultVersion(ctx context.Context, req *buddyv1.RestoreVaultVersionRequest) (*buddyv1.RestoreVaultVersionResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.deps.Vault.RestoreVaultVersion(ctx, p.UserID, req.GetItemId(), int(req.GetVersion()))
	if err != nil {
		return nil, storeError(err)
	}
	pv, err := vaultProto(v)
	if err != nil {
		return nil, status.Error(codes.Internal, "vault data")
	}
	return &buddyv1.RestoreVaultVersionResponse{Item: pv}, nil
}

func (s *vaultService) DeleteVaultItem(ctx context.Context, req *buddyv1.DeleteVaultItemRequest) (*buddyv1.DeleteVaultItemResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Vault.DeleteVaultItem(ctx, p.UserID, req.GetItemId()); err != nil {
		return nil, storeError(err)
	}
	return &buddyv1.DeleteVaultItemResponse{}, nil
}

// SaveMessageToVault keeps an answer as it is, with the answer itself as the source.
func (s *vaultService) SaveMessageToVault(ctx context.Context, req *buddyv1.SaveMessageToVaultRequest) (*buddyv1.SaveMessageToVaultResponse, error) {
	p, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	if s.deps.Pins == nil {
		return nil, status.Error(codes.Unimplemented, "messages are not available")
	}
	msg, err := s.deps.Pins.OwnMessage(ctx, p.UserID, req.GetMessageId())
	if err != nil {
		return nil, storeError(err)
	}
	text := strings.TrimSpace(markerStrip.ReplaceAllString(msg.Content, ""))
	title := strings.TrimSpace(req.GetTitle())
	if title == "" {
		title, _, _ = strings.Cut(text, "\n")
		title = strings.Trim(title, "#*_ ")
	}
	sources, err := json.Marshal([]map[string]string{{"kind": "message", "id": msg.ID, "title": cutRunes(title, maxVaultTitle)}})
	if err != nil {
		return nil, status.Error(codes.Internal, "vault data")
	}
	v, err := s.deps.Vault.CreateVaultItem(ctx, domain.VaultItem{
		UserID: p.UserID, Title: cutRunes(title, maxVaultTitle), Summary: cutRunes(text, maxVaultSummary), Sources: sources,
	})
	if err != nil {
		return nil, storeError(err)
	}
	pv, err := vaultProto(v)
	if err != nil {
		return nil, status.Error(codes.Internal, "vault data")
	}
	return &buddyv1.SaveMessageToVaultResponse{Item: pv}, nil
}

func vaultProto(v domain.VaultItem) (*buddyv1.VaultItem, error) {
	out := &buddyv1.VaultItem{
		Id: v.ID, Title: v.Title, Summary: v.Summary, Tags: v.Tags, Version: int32(v.Version), //nolint:gosec // versions are small
		UpdatedAt: v.UpdatedAt.UTC().Format(time.RFC3339), Sources: []*structpb.Struct{},
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if len(v.Sources) > 0 {
		var list []map[string]any
		if err := json.Unmarshal(v.Sources, &list); err != nil {
			return nil, err
		}
		for _, src := range list {
			st, err := structpb.NewStruct(src)
			if err != nil {
				return nil, err
			}
			out.Sources = append(out.Sources, st)
		}
	}
	for _, ver := range v.Versions {
		pv, err := vaultProto(ver)
		if err != nil {
			return nil, err
		}
		pv.UpdatedAt = ver.CreatedAt.UTC().Format(time.RFC3339)
		out.Versions = append(out.Versions, pv)
	}
	return out, nil
}
