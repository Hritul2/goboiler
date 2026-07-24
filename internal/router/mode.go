package router

import "github.com/gin-gonic/gin"

// ginMode maps cfg.AppConfig.Env (validated oneof=dev stage prod) onto
// gin's run mode. stage/prod run in ReleaseMode — debug mode prints its own
// unstructured lines straight to stdout, bypassing zerolog/OTel entirely,
// and gin itself warns it's unsuitable for production.
func ginMode(env string) string {
	switch env {
	case "prod", "stage":
		return gin.ReleaseMode
	default:
		return gin.DebugMode
	}
}
