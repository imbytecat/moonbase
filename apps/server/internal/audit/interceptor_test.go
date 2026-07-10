package audit

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/imbytecat/moonbase/server/internal/auth"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	userv1 "github.com/imbytecat/moonbase/server/internal/gen/user/v1"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

type fakeAuditQuerier struct {
	repository.Querier
	inserted []repository.InsertAuditLogParams
}

func (f *fakeAuditQuerier) InsertAuditLog(
	_ context.Context,
	arg repository.InsertAuditLogParams,
) error {
	f.inserted = append(f.inserted, arg)
	return nil
}

type fakeRequest struct {
	connect.AnyRequest
	procedure string
	msg       any
}

func newFakeRequest(procedure string, msg any) *fakeRequest {
	return &fakeRequest{procedure: procedure, msg: msg}
}

func (r *fakeRequest) Spec() connect.Spec {
	return connect.Spec{Procedure: r.procedure}
}

func (r *fakeRequest) Any() any { return r.msg }

func (r *fakeRequest) Peer() connect.Peer {
	return connect.Peer{Addr: "192.0.2.1:1234"}
}

func (r *fakeRequest) Header() http.Header {
	return http.Header{"User-Agent": {"test-agent"}}
}

func TestInterceptorRecordsMutations(t *testing.T) {
	repo := &fakeAuditQuerier{}
	userID := uuid.New()
	ctx := auth.WithIdentity(t.Context(), &auth.Identity{UserID: userID})

	interceptor := NewInterceptor(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	next := connect.UnaryFunc(
		func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			return connect.NewResponse(&userv1.DeleteUserResponse{}), nil
		},
	)
	targetID := uuid.NewString()
	_, err := interceptor(next)(ctx, newFakeRequest(
		"/user.v1.UserService/DeleteUser",
		&userv1.DeleteUserRequest{Id: targetID},
	))
	if err != nil {
		t.Fatal(err)
	}

	if len(repo.inserted) != 1 {
		t.Fatalf("inserted %d rows, want 1", len(repo.inserted))
	}
	got := repo.inserted[0]
	if got.Action != "/user.v1.UserService/DeleteUser" {
		t.Errorf("action = %q", got.Action)
	}
	if got.Domain != "user" {
		t.Errorf("domain = %q, want user", got.Domain)
	}
	if got.ResourceID != targetID {
		t.Errorf("resource_id = %q, want %q", got.ResourceID, targetID)
	}
	if got.Result != "ok" {
		t.Errorf("result = %q, want ok", got.Result)
	}
	if !got.ActorID.Valid || uuid.UUID(got.ActorID.Bytes) != userID {
		t.Errorf("actor_id = %v, want %s", got.ActorID, userID)
	}
	if got.Ip != "192.0.2.1" {
		t.Errorf("ip = %q", got.Ip)
	}
}

func TestInterceptorSkipsReads(t *testing.T) {
	repo := &fakeAuditQuerier{}
	interceptor := NewInterceptor(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	next := connect.UnaryFunc(
		func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			return connect.NewResponse(&userv1.ListUsersResponse{}), nil
		},
	)

	for _, procedure := range []string{
		"/user.v1.UserService/ListUsers",
		"/system.v1.SystemService/GetSystemSettings",
		"/auth.v1.AuthService/Logout",
	} {
		if _, err := interceptor(
			next,
		)(
			t.Context(),
			newFakeRequest(procedure, &userv1.ListUsersRequest{}),
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.inserted) != 0 {
		t.Fatalf("reads must not be recorded, got %d rows", len(repo.inserted))
	}
}

func TestInterceptorExtractsNestedProfileID(t *testing.T) {
	repo := &fakeAuditQuerier{}
	interceptor := NewInterceptor(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	next := connect.UnaryFunc(
		func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			return connect.NewResponse(&systemv1.UpdateCaptchaProfileResponse{}), nil
		},
	)

	_, err := interceptor(next)(t.Context(), newFakeRequest(
		"/system.v1.SystemService/UpdateCaptchaProfile",
		&systemv1.UpdateCaptchaProfileRequest{
			Profile: &systemv1.ProfileInput{Id: "prof-1", Provider: "altcha"},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.inserted) != 1 || repo.inserted[0].ResourceID != "prof-1" {
		t.Fatalf("nested profile id not extracted: %+v", repo.inserted)
	}
}

func TestInterceptorRecordsErrorResult(t *testing.T) {
	repo := &fakeAuditQuerier{}
	interceptor := NewInterceptor(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	next := connect.UnaryFunc(
		func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			return nil, connect.NewError(connect.CodePermissionDenied, nil)
		},
	)

	_, err := interceptor(next)(t.Context(), newFakeRequest(
		"/user.v1.UserService/DeleteUser",
		&userv1.DeleteUserRequest{Id: uuid.NewString()},
	))
	if err == nil {
		t.Fatal("expected the error to propagate")
	}
	if len(repo.inserted) != 1 || repo.inserted[0].Result != "permission_denied" {
		t.Fatalf("error result not recorded: %+v", repo.inserted)
	}
}
