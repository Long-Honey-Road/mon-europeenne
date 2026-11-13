// Package documents is the documents mock: the team wiki with synthetic training instructions from seed.
package documents

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	documentsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/documents/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

type Service struct {
	documentsv1.UnimplementedDocumentsServiceServer
	docs []seed.Doc
}

func New(docs []seed.Doc) *Service {
	return &Service{docs: docs}
}

func (s *Service) ListDocuments(_ context.Context, req *documentsv1.ListDocumentsRequest) (*documentsv1.ListDocumentsResponse, error) {
	out := &documentsv1.ListDocumentsResponse{}
	for _, d := range s.docs {
		if (req.GetTeamId() == "" || d.TeamID == req.GetTeamId()) && (req.GetTopic() == "" || d.Topic == req.GetTopic()) {
			out.Documents = append(out.Documents, info(d))
		}
	}
	return out, nil
}

func (s *Service) GetDocument(_ context.Context, req *documentsv1.GetDocumentRequest) (*documentsv1.GetDocumentResponse, error) {
	for _, d := range s.docs {
		if d.ID == req.GetDocumentId() {
			return &documentsv1.GetDocumentResponse{Info: info(d), Markdown: d.Markdown}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "document %q not found", req.GetDocumentId())
}

func info(d seed.Doc) *documentsv1.DocumentInfo {
	return &documentsv1.DocumentInfo{Id: d.ID, Title: d.Title, TeamId: d.TeamID, Topic: d.Topic, TestInstruction: true}
}
