package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const (
	maxVaultTitle   = 80
	maxVaultSummary = 1200
	maxVaultTags    = 8
)

// VaultStore keeps the user's private vault; every call is scoped to the caller.
type VaultStore interface {
	CreateVaultItem(ctx context.Context, v domain.VaultItem) (domain.VaultItem, error)
	VaultItems(ctx context.Context, userID, tag string) ([]domain.VaultItem, error)
	VaultItem(ctx context.Context, userID, id string) (domain.VaultItem, error)
	UpdateVaultItem(ctx context.Context, userID, id string, p domain.VaultPatch) (domain.VaultItem, error)
}

type VaultSource struct {
	Kind  string `json:"kind" jsonschema:"message, doc, repo or plan_item"`
	ID    string `json:"id" jsonschema:"the source id: source_id of a fragment, a plan item id, or message"`
	Title string `json:"title,omitempty"`
}

type vaultSaveIn struct {
	Title   string        `json:"title" jsonschema:"short title, up to 80 characters"`
	Summary string        `json:"summary" jsonschema:"markdown up to 1200 characters: the gist in one or two sentences, key facts as a list"`
	Tags    []string      `json:"tags,omitempty" jsonschema:"a few short tags"`
	Sources []VaultSource `json:"sources" jsonschema:"where it comes from; at least one"`
}

type vaultUpdateIn struct {
	ItemID  string   `json:"item_id" jsonschema:"the record id; the record in focus is in the prompt"`
	Title   string   `json:"title,omitempty"`
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type vaultListIn struct {
	Tag string `json:"tag,omitempty"`
}

type vaultItemOut struct {
	ItemID  string   `json:"item_id"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
	Version int      `json:"version"`
}

type vaultSavedOut struct {
	vaultItemOut
	UIEvent UIEvent `json:"ui_event"`
}

type vaultListOut struct {
	Items []vaultItemOut `json:"items"`
}

func itemOut(v domain.VaultItem) vaultItemOut {
	return vaultItemOut{ItemID: v.ID, Title: v.Title, Tags: orNoTags(v.Tags), Version: v.Version}
}

func orNoTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func savedEvent(v domain.VaultItem) UIEvent {
	return UIEvent{Type: "vault_saved", Data: map[string]any{"item_id": v.ID, "title": v.Title, "version": v.Version}}
}

// checkSummary is the server's check of a vault summary; the error tells the model what to fix.
func checkSummary(s string) error {
	n := len([]rune(strings.TrimSpace(s)))
	switch {
	case n == 0:
		return fmt.Errorf("%w: summary is empty", ErrInvalidArgs)
	case n > maxVaultSummary:
		return fmt.Errorf("%w: summary is %d characters, shorten it to %d", ErrInvalidArgs, n, maxVaultSummary)
	}
	return nil
}

func cleanTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" && len(out) < maxVaultTags {
			out = append(out, cutRunes(t, 30))
		}
	}
	return out
}

var vaultThreads = []domain.ThreadType{domain.ThreadVault}

// VaultSave saves a note to the user's vault, only when the user asked to.
func VaultSave(vault VaultStore) (Tool, error) {
	return Define("vault_save",
		"Save a short note to the user's private vault ONLY when the user asked in this message («сохрани в "+
			"хранилище»). Summarize the relevant messages and list where it comes from.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in vaultSaveIn) (vaultSavedOut, error) {
			title := cutRunes(in.Title, maxVaultTitle)
			if title == "" {
				return vaultSavedOut{}, fmt.Errorf("%w: title is required", ErrInvalidArgs)
			}
			if err := checkSummary(in.Summary); err != nil {
				return vaultSavedOut{}, err
			}
			if len(in.Sources) == 0 {
				return vaultSavedOut{}, fmt.Errorf("%w: at least one source is required", ErrInvalidArgs)
			}
			sources, err := json.Marshal(in.Sources)
			if err != nil {
				return vaultSavedOut{}, err
			}
			v, err := vault.CreateVaultItem(ctx, domain.VaultItem{
				UserID: p.UserID, Title: title, Summary: strings.TrimSpace(in.Summary), Tags: cleanTags(in.Tags), Sources: sources,
			})
			if err != nil {
				return vaultSavedOut{}, err
			}
			return vaultSavedOut{vaultItemOut: itemOut(v), UIEvent: savedEvent(v)}, nil
		})
}

// VaultUpdate changes a vault record as a new version, only on the user's request.
func VaultUpdate(vault VaultStore) (Tool, error) {
	return Define("vault_update",
		"Change a record of the user's vault (shorten, add, retitle) ONLY when the user asked for it; every change is "+
			"a new version the user can roll back.",
		allRoles, vaultThreads,
		func(ctx context.Context, p domain.Principal, in vaultUpdateIn) (vaultSavedOut, error) {
			var patch domain.VaultPatch
			if t := cutRunes(in.Title, maxVaultTitle); t != "" {
				patch.Title = &t
			}
			if strings.TrimSpace(in.Summary) != "" {
				if err := checkSummary(in.Summary); err != nil {
					return vaultSavedOut{}, err
				}
				s := strings.TrimSpace(in.Summary)
				patch.Summary = &s
			}
			if in.Tags != nil {
				patch.Tags = cleanTags(in.Tags)
			}
			if patch.Title == nil && patch.Summary == nil && patch.Tags == nil {
				return vaultSavedOut{}, fmt.Errorf("%w: nothing to change", ErrInvalidArgs)
			}
			v, err := vault.UpdateVaultItem(ctx, p.UserID, in.ItemID, patch)
			if err != nil {
				return vaultSavedOut{}, err
			}
			return vaultSavedOut{vaultItemOut: itemOut(v), UIEvent: savedEvent(v)}, nil
		})
}

// VaultList shows the user's vault records, titles and tags only.
func VaultList(vault VaultStore) (Tool, error) {
	return Define("vault_list",
		"List the user's private vault records (titles and tags), e.g. to find one they mean.",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in vaultListIn) (vaultListOut, error) {
			items, err := vault.VaultItems(ctx, p.UserID, strings.ToLower(strings.TrimSpace(in.Tag)))
			if err != nil {
				return vaultListOut{}, err
			}
			out := vaultListOut{Items: []vaultItemOut{}}
			for _, v := range items {
				out.Items = append(out.Items, itemOut(v))
			}
			return out, nil
		})
}
