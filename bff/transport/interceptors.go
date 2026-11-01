package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect/v2"

	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/ports"
)

// masking is the outermost interceptor, so nothing else can return an unmapped error. It also turns
// a panic into the same internal error.
func masking(logger *slog.Logger) connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = internal(ctx, logger, spec, fmt.Errorf("panic: %v", recovered))
				}
			}()
			return contractError(ctx, logger, spec, next(ctx, spec, stream))
		}
	}
}

// downstream marks an error that crossed the store or authenticator boundary. Code on the far side
// is not trusted to decide what a caller may read, so a *connect.Error it returns is never public,
// whatever its code. Only the sentinels in contractError keep their meaning across this boundary.
type downstream struct{ cause error }

func (d downstream) Error() string { return d.cause.Error() }

func (d downstream) Unwrap() error { return d.cause }

func fromDownstream(err error) error {
	if err == nil {
		return nil
	}
	return downstream{err}
}

// contractError is the fixed mapping in specs/001 contracts/notes-api.md. What reaches a caller is
// one of these constants, or a *connect.Error from transport or the framework, which say only what
// the caller sent. Anything else is internal.
func contractError(ctx context.Context, logger *slog.Logger, spec connect.Spec, err error) error {
	var public *connect.Error
	var untrusted downstream
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ports.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, "note not found")
	case errors.Is(err, identity.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, "authentication is required")
	case errors.Is(err, ports.ErrUnavailable):
		return connect.NewError(connect.CodeUnavailable, "the service is unavailable; try again")
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, "the request took too long")
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, "the request was cancelled")
	case errors.As(err, &untrusted):
		return internal(ctx, logger, spec, err)
	case errors.As(err, &public) && public.Code() != connect.CodeInternal && public.Code() != connect.CodeUnknown:
		return public
	default:
		return internal(ctx, logger, spec, err)
	}
}

func internal(ctx context.Context, logger *slog.Logger, spec connect.Spec, cause error) error {
	var raw [8]byte
	_, _ = rand.Read(raw[:]) // crypto/rand.Read never returns an error.
	id := hex.EncodeToString(raw[:])
	logger.ErrorContext(ctx, "internal error", "correlation_id", id, "procedure", spec.Procedure, "error", cause.Error())
	return connect.NewError(connect.CodeInternal, "internal error, correlation id "+id)
}

// bounded gives every call a deadline. A client's shorter deadline stays in force.
func bounded(limits Limits) connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			limit := limits.MaxDeadline
			if _, ok := ctx.Deadline(); !ok {
				limit = limits.DefaultDeadline
			}
			ctx, cancel := context.WithTimeout(ctx, limit)
			defer cancel()
			return next(ctx, spec, stream)
		}
	}
}

// authenticating runs before the request is decoded. It reads only the Authorization header.
func authenticating(authenticator identity.Authenticator, allowTestIdentity bool) connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			info, ok := connect.CallInfoForServerContext(ctx)
			if !ok {
				return identity.ErrUnauthenticated
			}
			principal, err := authenticator.Authenticate(ctx, info.RequestHeader().Get("Authorization"))
			if err != nil {
				return fromDownstream(err)
			}
			if principal.Kind == identity.KindTestIdentity && !allowTestIdentity {
				return identity.ErrUnauthenticated
			}
			return next(identity.WithPrincipal(ctx, principal), spec, stream)
		}
	}
}
