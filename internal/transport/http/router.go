package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/example/pulse-backend/internal/app/adminauth"
	appauth "github.com/example/pulse-backend/internal/app/auth"
	appchat "github.com/example/pulse-backend/internal/app/chat"
	appdiscovery "github.com/example/pulse-backend/internal/app/discovery"
	appmatching "github.com/example/pulse-backend/internal/app/matching"
	appmedia "github.com/example/pulse-backend/internal/app/media"
	appmoderation "github.com/example/pulse-backend/internal/app/moderation"
	appprofile "github.com/example/pulse-backend/internal/app/profile"
	httpauth "github.com/example/pulse-backend/internal/transport/http/auth"
	httpchat "github.com/example/pulse-backend/internal/transport/http/chat"
	httpdiscovery "github.com/example/pulse-backend/internal/transport/http/discovery"
	"github.com/example/pulse-backend/internal/transport/http/health"
	httpmatching "github.com/example/pulse-backend/internal/transport/http/matching"
	httpmedia "github.com/example/pulse-backend/internal/transport/http/media"
	"github.com/example/pulse-backend/internal/transport/http/middleware"
	httpmoderation "github.com/example/pulse-backend/internal/transport/http/moderation"
	httpprofile "github.com/example/pulse-backend/internal/transport/http/profile"
)

// Dependencies содержит зависимости всех HTTP-обработчиков. Конкретные
// реализации собираются в cmd/api, а не создаются внутри transport-слоя.
type Dependencies struct {
	Logger     *slog.Logger
	Database   health.Pinger
	Auth       appauth.Service
	AdminAuth  adminauth.Service
	Profile    appprofile.Service
	Media      appmedia.Service
	Discovery  appdiscovery.Service
	Matching   appmatching.Service
	Chat       appchat.Service
	Moderation appmoderation.Service
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	metrics := middleware.NewMetrics()

	health.RegisterRoutes(mux, health.NewHandler(deps.Database))
	mux.Handle("GET /metrics", metrics)

	httpauth.RegisterRoutes(mux, httpauth.NewHandler(deps.Auth, deps.AdminAuth))
	httpprofile.RegisterRoutes(mux, httpprofile.NewHandler(deps.Profile))
	httpmedia.RegisterRoutes(mux, httpmedia.NewHandler(deps.Media))
	httpdiscovery.RegisterRoutes(mux, httpdiscovery.NewHandler(deps.Discovery))
	httpmatching.RegisterRoutes(mux, httpmatching.NewHandler(deps.Matching))
	httpchat.RegisterRoutes(mux, httpchat.NewHandler(deps.Chat))
	httpmoderation.RegisterRoutes(mux, httpmoderation.NewHandler(deps.Moderation))

	return middleware.Recover(
		deps.Logger,
		metrics.Count(middleware.Logging(deps.Logger, mux)),
	)
}
