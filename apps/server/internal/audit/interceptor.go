// Package audit records mutating RPCs into the append-only audit_logs table
// via a Connect interceptor. Capture lives at ONE seam (like authz) so no
// handler can forget it. Request payloads are never stored — system-settings
// secrets stay write-only even here; only the procedure, actor, result and a
// best-effort resource id are kept.
package audit

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/imbytecat/moonbase/server/internal/auth"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

// readPrefixes mark procedures that never mutate — they are not recorded.
// Everything else (Create/Update/Delete/Bind/Send/Test/Login/...) is.
var readPrefixes = []string{"Get", "List"}

// skippedProcedures are high-frequency or zero-value entries: session
// touches and pure reads that a trail of would only be noise.
var skippedProcedures = map[string]bool{
	"/auth.v1.AuthService/Logout": true,
}

func recordable(procedure string) bool {
	if skippedProcedures[procedure] {
		return false
	}
	method := procedure[strings.LastIndex(procedure, "/")+1:]
	for _, p := range readPrefixes {
		if strings.HasPrefix(method, p) {
			return false
		}
	}
	return true
}

// domainOf extracts the proto package short name: "/user.v1.UserService/X"
// → "user".
func domainOf(procedure string) string {
	rest := strings.TrimPrefix(procedure, "/")
	pkg, _, _ := strings.Cut(rest, ".")
	return pkg
}

// NewInterceptor records every mutating unary RPC after it completes.
// Recording is best-effort: an insert failure is logged, never surfaced —
// the audit trail must not take the product down.
func NewInterceptor(repo repository.Querier, logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			procedure := req.Spec().Procedure
			if !recordable(procedure) {
				return res, err
			}

			var actorID pgtype.UUID
			if id := auth.IdentityFromContext(ctx); id != nil {
				actorID = pgtype.UUID{Bytes: id.UserID, Valid: true}
			}
			result := "ok"
			if err != nil {
				result = connect.CodeOf(err).String()
			}
			ip, _, splitErr := net.SplitHostPort(req.Peer().Addr)
			if splitErr != nil {
				ip = req.Peer().Addr
			}
			if insertErr := repo.InsertAuditLog(ctx, repository.InsertAuditLogParams{
				ActorID:    actorID,
				Action:     procedure,
				Domain:     domainOf(procedure),
				ResourceID: resourceID(req.Any()),
				Result:     result,
				Ip:         ip,
				UserAgent:  userAgent(req.Header()),
			}); insertErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "audit log insert failed",
					"procedure", procedure, "error", insertErr)
			}
			return res, err
		}
	}
}

// resourceID pulls the target entity id from the request without storing the
// payload: a top-level "id" string field, or the "id" of a top-level message
// field (the Update*Profile shape). Anything else records empty.
func resourceID(msg any) string {
	pm, ok := msg.(proto.Message)
	if !ok {
		return ""
	}
	m := pm.ProtoReflect()
	if id := stringField(m, "id"); id != "" {
		return id
	}
	var nested string
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			if id := stringField(v.Message(), "id"); id != "" {
				nested = id
				return false
			}
		}
		return true
	})
	return nested
}

func stringField(m protoreflect.Message, name string) string {
	fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil || fd.Kind() != protoreflect.StringKind || fd.IsList() {
		return ""
	}
	return m.Get(fd).String()
}

func userAgent(h http.Header) string {
	ua := h.Get("User-Agent")
	if len(ua) > 256 {
		ua = ua[:256]
	}
	return ua
}
