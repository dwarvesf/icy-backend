package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dwarvesf/icy-backend/internal/utils/config"
	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// configureTrustedProxies decides whether X-Forwarded-For may be believed.
//
// gin defaults to ForwardedByClientIP with trustedCIDRs of 0.0.0.0/0 and ::/0,
// so out of the box ClientIP() returns the leftmost X-Forwarded-For entry from
// ANY source. That makes every per-IP control decorative: an attacker rotates a
// header to get unlimited buckets, or pins a victim's IP to drain theirs. It
// also lets one socket create unbounded limiter entries.
//
// TRUSTED_PROXIES is a comma-separated CIDR list of the hops actually in front
// of this service. Empty means "nothing is in front", which is the safe
// default: ClientIP() then returns the real socket peer and the header is
// ignored entirely.
func configureTrustedProxies(r *gin.Engine, cfg *config.AppConfig, log *logger.Logger) {
	raw := strings.TrimSpace(cfg.ApiServer.TrustedProxies)

	if raw == "" {
		// nil (not empty slice) disables header-derived client IPs entirely.
		if err := r.SetTrustedProxies(nil); err != nil && log != nil {
			log.Error("[configureTrustedProxies][SetTrustedProxies]", map[string]string{
				"error": err.Error(),
			})
		}
		if log != nil {
			log.Info("[configureTrustedProxies] no TRUSTED_PROXIES set, using socket peer as client IP and ignoring X-Forwarded-For")
		}
		return
	}

	var cidrs []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cidrs = append(cidrs, p)
		}
	}

	if err := r.SetTrustedProxies(cidrs); err != nil {
		// Fail CLOSED: a malformed list must not silently leave gin trusting
		// everything, which is exactly the state this function exists to end.
		if log != nil {
			log.Error("[configureTrustedProxies][SetTrustedProxies] invalid TRUSTED_PROXIES, falling back to trusting none", map[string]string{
				"error": err.Error(),
			})
		}
		_ = r.SetTrustedProxies(nil)
		return
	}

	if log != nil {
		log.Info("[configureTrustedProxies] trusting " + strings.Join(cidrs, ", "))
	}
}
