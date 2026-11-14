package router

import (
	"github.com/hritul2/goboiler/internal/api"
	"github.com/hritul2/goboiler/internal/handler"
	"github.com/hritul2/goboiler/internal/handler/user"
)

// APIVersions is the single registry of RPC API versions. It feeds BOTH the
// router (router.New mounts each entry as its own gin group) and the
// TypeScript generator (cmd/gen writes each entry to gen/ts/<Name>/), so the
// served API and the generated client cannot drift.
//
// To add a function: append its endpoint here (usually via a domain's
// Endpoints(h) helper). To add /api/v2: append a second api.Version with
// Name "v2" and Prefix "/api/v2/rpc". Versions are independent — each gets
// its own route group, its own middleware and its own generated folder — so
// v2 can reuse v1's handlers, wrap them, or diverge entirely without
// touching v1.
//
// Nothing here knows about webhooks: those are plain gin routes under
// /webhook/<provider> (see router.New) and are never generated.
func APIVersions(h *handler.Handlers) []api.Version {
	return []api.Version{
		{
			Name:      "v1",
			Prefix:    "/api/v1/rpc",
			Endpoints: user.Endpoints(h.User),
		},
	}
}
