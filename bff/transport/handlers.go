// Package transport adapts the generated contract to the use cases. It owns validation, the error
// mapping, deadlines, the authentication boundary and CORS.
package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"buf.build/go/protovalidate"
	connect "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	notesv1 "example.com/bfftemplate/api/template/notes/v1"
	"example.com/bfftemplate/api/template/notes/v1/notesv1connect"
	"example.com/bfftemplate/bff/app"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/ports"
)

const (
	// DefaultDeadline applies when the client sets no deadline.
	DefaultDeadline = 2 * time.Second
	// MaxDeadline caps any deadline a client asks for.
	MaxDeadline = 10 * time.Second
	// MaxRequestBytes fits a maximum note and is enforced before decoding.
	MaxRequestBytes = 64 << 10
)

// Options configures the handler.
type Options struct {
	Notes             *app.Notes
	Authenticator     identity.Authenticator
	AllowTestIdentity bool
	AllowedOrigin     string
	Logger            *slog.Logger
}

// NewHandler returns the HTTP handler for the notes service.
func NewHandler(options Options) http.Handler {
	server := connect.NewServer(
		masking(options.Logger),
		bounded(),
		authenticating(options.Authenticator, options.AllowTestIdentity),
	)
	notesv1connect.RegisterNoteServiceHandler(server, notes{app: options.Notes})
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server, connecthttp.WithReadMaxBytes(MaxRequestBytes))
	return cors(options.AllowedOrigin, mux)
}

type notes struct{ app *app.Notes }

func (n notes) CreateNote(ctx context.Context, req *notesv1.CreateNoteRequest) (*notesv1.CreateNoteResponse, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := n.app.Create(ctx, principal.Subject, req.GetNote().GetTitle(), req.GetNote().GetBody())
	if err != nil {
		return nil, fromDownstream(err)
	}
	return &notesv1.CreateNoteResponse{Note: toProto(note)}, nil
}

func (n notes) GetNote(ctx context.Context, req *notesv1.GetNoteRequest) (*notesv1.GetNoteResponse, error) {
	if err := validate(req); err != nil {
		return nil, err
	}
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := n.app.Get(ctx, principal.Subject, req.GetName())
	if err != nil {
		return nil, fromDownstream(err)
	}
	return &notesv1.GetNoteResponse{Note: toProto(note)}, nil
}

func caller(ctx context.Context) (identity.Principal, error) {
	principal, ok := identity.FromContext(ctx)
	if !ok {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	return principal, nil
}

func toProto(note ports.Note) *notesv1.Note {
	return &notesv1.Note{
		Name:       app.Name(note.ID),
		Title:      note.Title,
		Body:       note.Body,
		CreateTime: timestamppb.New(note.CreateTime),
	}
}

func validate(message proto.Message) error {
	err := protovalidate.Validate(message)
	var invalid *protovalidate.ValidationError
	if !errors.As(err, &invalid) {
		return err
	}
	detail, detailErr := connectproto.NewErrorDetail(invalid.ToProto())
	if detailErr != nil {
		return fmt.Errorf("encode violations: %w", detailErr)
	}
	return connect.NewError(connect.CodeInvalidArgument, "the request is invalid").WithDetail(detail)
}
