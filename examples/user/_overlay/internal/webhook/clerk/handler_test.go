package clerk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hritul2/goboiler/internal/middleware"
)

const testSecret = "whsec_" + "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw" // base64 text; any valid base64 works

type fakeSyncer struct {
	calls []string
	err   error
}

func (f *fakeSyncer) SyncFromClerk(_ context.Context, id, email, name string) error {
	f.calls = append(f.calls, id+"|"+email+"|"+name)
	return f.err
}

// sign produces a valid svix-signature header value for body.
func sign(t *testing.T, id string, ts time.Time, body string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(testSecret, "whsec_"))
	require.NoError(t, err)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(ts.Unix(), 10) + "." + body))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newEngine(t *testing.T, svc userSyncer, now time.Time) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h, err := NewHandler(testSecret, svc)
	require.NoError(t, err)
	h.now = func() time.Time { return now }

	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Group("/webhook").POST("/clerk", middleware.MaxReqResSizeOverride(1<<20), h.Handle)
	return engine
}

func deliver(engine *gin.Engine, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook/clerk", strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

const userCreatedBody = `{"type":"user.created","data":{"id":"user_2abc","first_name":"Ada","last_name":"Lovelace",` +
	`"primary_email_address_id":"idn_2","email_addresses":[{"id":"idn_1","email_address":"old@example.com"},` +
	`{"id":"idn_2","email_address":"ada@example.com"}]}}`

func TestUserCreated_SyncsUser(t *testing.T) {
	now := time.Now()
	svc := &fakeSyncer{}
	w := deliver(newEngine(t, svc, now), userCreatedBody, map[string]string{
		"svix-id":        "msg_1",
		"svix-timestamp": strconv.FormatInt(now.Unix(), 10),
		"svix-signature": sign(t, "msg_1", now, userCreatedBody),
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"user_2abc|ada@example.com|Ada Lovelace"}, svc.calls)
}

func TestSignatureRejections(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	good := sign(t, "msg_1", now, userCreatedBody)

	tests := map[string]struct {
		body string
		hdr  map[string]string
	}{
		"missing headers":    {userCreatedBody, nil},
		"tampered body":      {strings.Replace(userCreatedBody, "Ada", "Eve", 1), map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": good}},
		"wrong id":           {userCreatedBody, map[string]string{"svix-id": "msg_2", "svix-timestamp": ts, "svix-signature": good}},
		"garbage signature":  {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": "v1,!!!"}},
		"unknown version":    {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": ts, "svix-signature": strings.Replace(good, "v1,", "v2,", 1)}},
		"replay (too old)":   {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10), "svix-signature": sign(t, "msg_1", now.Add(-10*time.Minute), userCreatedBody)}},
		"from the future":    {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": strconv.FormatInt(now.Add(10*time.Minute).Unix(), 10), "svix-signature": sign(t, "msg_1", now.Add(10*time.Minute), userCreatedBody)}},
		"bad timestamp text": {userCreatedBody, map[string]string{"svix-id": "msg_1", "svix-timestamp": "yesterday", "svix-signature": good}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			svc := &fakeSyncer{}
			w := deliver(newEngine(t, svc, now), tt.body, tt.hdr)
			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			assert.Empty(t, svc.calls, "an unverified event must never reach the service")
		})
	}
}

func TestRotatedSecret_AnyMatchingSignatureWins(t *testing.T) {
	now := time.Now()
	hdr := map[string]string{
		"svix-id":        "msg_1",
		"svix-timestamp": strconv.FormatInt(now.Unix(), 10),
		"svix-signature": "v1,AAAA " + sign(t, "msg_1", now, userCreatedBody), // old key first, current key second
	}
	w := deliver(newEngine(t, &fakeSyncer{}, now), userCreatedBody, hdr)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUnknownEventAcknowledged_ServiceErrorRetried(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	other := `{"type":"session.created","data":{}}`

	svc := &fakeSyncer{}
	w := deliver(newEngine(t, svc, now), other, map[string]string{"svix-id": "m", "svix-timestamp": ts, "svix-signature": sign(t, "m", now, other)})
	assert.Equal(t, http.StatusOK, w.Code, "unknown types are 200 so Svix does not retry them")
	assert.Empty(t, svc.calls)

	failing := &fakeSyncer{err: assert.AnError}
	w = deliver(newEngine(t, failing, now), userCreatedBody, map[string]string{"svix-id": "m", "svix-timestamp": ts, "svix-signature": sign(t, "m", now, userCreatedBody)})
	assert.Equal(t, http.StatusInternalServerError, w.Code, "a failed sync is non-2xx so Svix retries")
}

func TestNewHandler_RejectsBadSecret(t *testing.T) {
	_, err := NewHandler("whsec_***not-base64***", &fakeSyncer{})
	assert.Error(t, err)
}
