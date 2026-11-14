// Package clerk receives Clerk webhooks (delivered by Svix) and syncs users
// into our own store. Webhooks are NOT RPC functions: Clerk calls us, the
// signature is computed over the raw body, and the payload is Clerk's.
package clerk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	errs "github.com/hritul2/goboiler/internal/errors"
)

// maxSkew is how far the Svix timestamp may be from our clock. It bounds how
// long a captured request stays replayable.
const maxSkew = 5 * time.Minute

var errBadSignature = errors.New("clerk webhook: invalid signature")

// userSyncer is the narrow service interface the handler needs. Make the
// implementation idempotent (an upsert keyed by the Clerk user ID), because
// Svix delivers at least once: a retry or duplicate must not create a second
// user or fail.
type userSyncer interface {
	SyncFromClerk(ctx context.Context, clerkID, email, name string) error
}

// Handler serves POST /webhook/clerk.
type Handler struct {
	key []byte // decoded signing secret
	svc userSyncer
	now func() time.Time
}

// NewHandler takes the endpoint's signing secret as shown in the Clerk
// dashboard ("whsec_..."), which should come from config, not source.
func NewHandler(signingSecret string, svc userSyncer) (*Handler, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(signingSecret, "whsec_"))
	if err != nil || len(key) == 0 {
		return nil, fmt.Errorf("clerk webhook: signing secret is not a valid whsec_ value")
	}
	return &Handler{key: key, svc: svc, now: time.Now}, nil
}

// event is the envelope; Data is decoded per type.
type event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type userCreated struct {
	ID                    string `json:"id"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	PrimaryEmailAddressID string `json:"primary_email_address_id"`
	EmailAddresses        []struct {
		ID           string `json:"id"`
		EmailAddress string `json:"email_address"`
	} `json:"email_addresses"`
}

// Handle verifies the signature over the RAW body, then dispatches on the
// event type. Any non-2xx makes Svix retry; unknown event types are
// acknowledged with 200 so they are not retried forever.
func (h *Handler) Handle(c *gin.Context) {
	raw, err := c.GetRawData() // body exactly as sent; oversized => *http.MaxBytesError => 413
	if err != nil {
		_ = c.Error(err)
		return
	}
	if err := h.verify(raw, c.Request.Header); err != nil {
		_ = c.Error(errs.Unauthorized("invalid signature"))
		return
	}

	var evt event
	if err := json.Unmarshal(raw, &evt); err != nil {
		_ = c.Error(errs.BadRequest("invalid event"))
		return
	}

	switch evt.Type {
	case "user.created":
		var u userCreated
		if err := json.Unmarshal(evt.Data, &u); err != nil || u.ID == "" {
			_ = c.Error(errs.BadRequest("invalid user.created payload"))
			return
		}
		email := ""
		for _, e := range u.EmailAddresses {
			if e.ID == u.PrimaryEmailAddressID {
				email = e.EmailAddress
			}
		}
		name := strings.TrimSpace(u.FirstName + " " + u.LastName)
		if err := h.svc.SyncFromClerk(c.Request.Context(), u.ID, email, name); err != nil {
			_ = c.Error(err) // 5xx => Svix retries later
			return
		}
	default:
		// user.updated, user.deleted, ... : add a case when you need it.
	}
	c.Status(http.StatusOK)
}

// verify implements Svix's scheme: HMAC-SHA256 over "<svix-id>.<svix-timestamp>.<body>"
// with the decoded secret, compared in constant time against every "v1,<base64>"
// entry in the space-separated svix-signature header (several can be present
// during secret rotation). The timestamp check rejects replays.
func (h *Handler) verify(body []byte, hdr http.Header) error {
	id, ts, sigs := hdr.Get("svix-id"), hdr.Get("svix-timestamp"), hdr.Get("svix-signature")
	if id == "" || ts == "" || sigs == "" {
		return errBadSignature
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadSignature
	}
	if skew := h.now().Sub(time.Unix(sec, 0)); skew > maxSkew || skew < -maxSkew {
		return errBadSignature
	}

	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)

	for _, part := range strings.Fields(sigs) {
		version, sig, ok := strings.Cut(part, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(got, want) {
			return nil
		}
	}
	return errBadSignature
}
