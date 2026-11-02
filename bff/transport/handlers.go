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

// Limits are the call bounds a composition declares once (spec 011 FR-016); the handler carries no default
// of its own, and Check refuses a value outside the invariants below, whatever configuration asked for it.
type Limits struct {
	// DefaultDeadline applies to a call that sets no deadline.
	DefaultDeadline time.Duration
	// MaxDeadline caps any deadline a client asks for.
	MaxDeadline time.Duration
	// MaxRequestBytes is enforced before decoding.
	MaxRequestBytes int
}

// The invariants no configuration can weaken: a deadline beyond a minute would let a hung call hold a
// worker, and a request body must hold the schema's largest note yet never an unbounded one.
const (
	deadlineCeiling     = time.Minute
	requestBytesFloor   = 8 << 10
	requestBytesCeiling = 1 << 20
)

// Check refuses a missing bound, an inverted pair or a value outside the invariants, naming the field.
func (l Limits) Check() error {
	switch {
	case l.DefaultDeadline <= 0:
		return errors.New("calls.default_deadline: required and positive")
	case l.MaxDeadline <= 0:
		return errors.New("calls.max_deadline: required and positive")
	case l.DefaultDeadline > l.MaxDeadline:
		return fmt.Errorf("calls.default_deadline: %s exceeds calls.max_deadline %s", l.DefaultDeadline, l.MaxDeadline)
	case l.MaxDeadline > deadlineCeiling:
		return fmt.Errorf("calls.max_deadline: %s exceeds the ceiling %s; the ceiling is an invariant, not a setting", l.MaxDeadline, deadlineCeiling)
	case l.MaxRequestBytes < requestBytesFloor:
		return fmt.Errorf("calls.max_request_bytes: %d is below %d, which the schema's largest note needs", l.MaxRequestBytes, requestBytesFloor)
	case l.MaxRequestBytes > requestBytesCeiling:
		return fmt.Errorf("calls.max_request_bytes: %d exceeds the ceiling %d; the ceiling is an invariant, not a setting", l.MaxRequestBytes, requestBytesCeiling)
	}
	return nil
}

// Options configures the handler. Limits must have passed Check.
type Options struct {
	Telemetry         connect.ServerInterceptor
	Notes             *app.Notes
	Authenticator     identity.Authenticator
	AllowTestIdentity bool
	AllowedOrigin     string
	Logger            *slog.Logger
	Limits            Limits
}

// NewHandler returns the HTTP handler for the notes service.
func NewHandler(options Options) http.Handler {
	interceptors := []connect.ServerInterceptor{masking(options.Logger), bounded(options.Limits), authenticating(options.Authenticator, options.AllowTestIdentity)}
	if options.Telemetry != nil {
		interceptors = append([]connect.ServerInterceptor{options.Telemetry}, interceptors...)
	}
	server := connect.NewServer(interceptors...)
	notesv1connect.RegisterNoteServiceHandler(server, notes{app: options.Notes})
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server, connecthttp.WithReadMaxBytes(options.Limits.MaxRequestBytes))
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
