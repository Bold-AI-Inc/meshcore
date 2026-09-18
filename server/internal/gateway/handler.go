package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"mesh-server/internal/httputil"
	"mesh-server/internal/memstate"
	"mesh-server/internal/providerfamily"
	"mesh-server/internal/reqqueue"
	"mesh-server/internal/security"
)

const streamIdleTimeout = 300 * time.Second

type Options struct {
	Queue                      *reqqueue.Queue
	MaxRequestBodyBytes        int64
	QueueAdmitTimeout          time.Duration
	GenerationTimeout          time.Duration
	EmbeddingsTimeout          time.Duration
	MaxGenerationResponseBytes int64
	TrustProxyHeader           bool
}

type Handler struct {
	mem           *memstate.Store
	telemetry     *Telemetry
	meshKeyPepper string
	httpClient    *http.Client

	queue                      *reqqueue.Queue
	maxRequestBodyBytes        int64
	queueAdmitTimeout          time.Duration
	generationTimeout          time.Duration
	embeddingsTimeout          time.Duration
	maxGenerationResponseBytes int64
	trustProxyHeader           bool
}

func NewHandler(mem *memstate.Store, telemetry *Telemetry, meshKeyPepper string, opts Options) *Handler {
	return &Handler{
		mem:           mem,
		telemetry:     telemetry,
		meshKeyPepper: meshKeyPepper,
		httpClient:    newUpstreamClient(),

		queue:                      opts.Queue,
		maxRequestBodyBytes:        opts.MaxRequestBodyBytes,
		queueAdmitTimeout:          opts.QueueAdmitTimeout,
		generationTimeout:          opts.GenerationTimeout,
		embeddingsTimeout:          opts.EmbeddingsTimeout,
		maxGenerationResponseBytes: opts.MaxGenerationResponseBytes,
		trustProxyHeader:           opts.TrustProxyHeader,
	}
}

var errUnsafeRedirect = errors.New("upstream redirected to a non-routable address")

func safeRedirectTarget(ctx context.Context, u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return errUnsafeRedirect
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() || ip.IP.IsLinkLocalMulticast() || ip.IP.IsUnspecified() {
			return errUnsafeRedirect
		}
	}
	return nil
}

func newUpstreamClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("stopped after 3 redirects")
			}
			return safeRedirectTarget(req.Context(), req.URL)
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          512,
			MaxIdleConnsPerHost:   256,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
			WriteBufferSize:       64 << 10,
			ReadBufferSize:        64 << 10,
		},
	}
}

func newRequestID() string {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		id = ulid.ULID{}
	}
	return "req_" + id.String()
}

func (h *Handler) logsEnabled(providerID string) bool {
	if providerID == "" {
		return true
	}
	return !h.mem.Load().SilentProviders[providerID]
}

func (h *Handler) deny(w http.ResponseWriter, r *http.Request, start time.Time, resolved *memstate.ResolvedKey, providerID, modelID, requestedModel string, status int, code string, extra map[string]any) {
	writeError(w, status, code, extra)

	if !h.logsEnabled(providerID) {
		return
	}

	var keyID, userID, provIDPtr, modelIDPtr, reqModelPtr *string
	if resolved != nil {
		keyID = &resolved.KeyID
		userID = &resolved.UserID
	}
	if providerID != "" {
		provIDPtr = &providerID
	}
	if modelID != "" {
		modelIDPtr = &modelID
	}
	if requestedModel != "" {
		reqModelPtr = &requestedModel
	}
	var sourceIP *string
	if ip := httputil.ClientIP(r, h.trustProxyHeader); ip != "" {
		sourceIP = &ip
	}
	var userAgent *string
	if ua := r.UserAgent(); ua != "" {
		userAgent = &ua
	}

	logID, err := uuid.NewV7()
	if err != nil {
		return
	}
	denyReason := code
	h.telemetry.Enqueue(logEvent{
		ID:             logID,
		MeshKeyID:      keyID,
		UserID:         userID,
		ProviderID:     provIDPtr,
		ModelID:        modelIDPtr,
		RequestedModel: reqModelPtr,
		Outcome:        "denied",
		DenyReason:     &denyReason,
		SourceIP:       sourceIP,
		UserAgent:      userAgent,
		StatusCode:     status,
		LatencyMs:      int(time.Since(start).Milliseconds()),
	})
}

func (h *Handler) authenticate(r *http.Request) (resolved memstate.ResolvedKey, ok bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return memstate.ResolvedKey{}, false
	}
	keyHash := security.HashMeshKeyRaw(h.meshKeyPepper, token)
	resolved, ok = h.mem.Load().KeyIndex[keyHash]
	return resolved, ok
}

func checkUserStatus(resolved memstate.ResolvedKey, now time.Time) (code string, ok bool) {
	if resolved.UserStatus != "active" {
		return errInvalidOrRevokedKey, false
	}
	if resolved.UserExpiresAt != nil && now.After(*resolved.UserExpiresAt) {
		return errInvalidOrRevokedKey, false
	}
	if resolved.KeyExpiresAt != nil && now.After(*resolved.KeyExpiresAt) {
		return errInvalidOrRevokedKey, false
	}
	return "", true
}

func (h *Handler) policyDenial(now time.Time, userID string, access memstate.ModelAccess) (status int, code string, extra map[string]any) {
	if access.Provider.MaxCallsPerHour != nil {
		if h.mem.ProviderHourlyCount(access.Provider.ID) >= int64(*access.Provider.MaxCallsPerHour) {
			resumeAt := now.Truncate(time.Hour).Add(time.Hour)
			return http.StatusTooManyRequests, errProviderHourlyLimit, map[string]any{"resume_at": resumeAt.Format(time.RFC3339)}
		}
	}

	if access.Policy.MaxCallsPerHour != nil {
		if h.mem.UserModelHourlyCount(userID, access.ModelID) >= int64(*access.Policy.MaxCallsPerHour) {
			resumeAt := now.Truncate(time.Hour).Add(time.Hour)
			return http.StatusTooManyRequests, errUserModelHourlyLimit, map[string]any{
				"resume_at": resumeAt.Format(time.RFC3339),
				"limit":     *access.Policy.MaxCallsPerHour,
			}
		}
	}

	if !access.Policy.AlwaysOpen {
		if resumeAt, withinWindow := checkTimeWindow(now, access.Policy); !withinWindow {
			return http.StatusForbidden, errOutsideAllowedHours, map[string]any{"resume_at": resumeAt.Format(time.RFC3339)}
		}
	}

	if access.Policy.DailyCapUSD > 0 && h.mem.DailySpendUSD(userID, access.ModelID) >= access.Policy.DailyCapUSD {
		tomorrow := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
		return http.StatusTooManyRequests, errBudgetExceeded, map[string]any{
			"cap":       access.Policy.DailyCapUSD,
			"resume_at": tomorrow.Format(time.RFC3339),
		}
	}

	if h.mem.KillSwitchTripped() {
		return http.StatusServiceUnavailable, errPausedByAdmin, nil
	}

	return 0, "", nil
}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestedModel := r.URL.Query().Get("model")
	requestID := newRequestID()
	w.Header().Set("X-Request-Id", requestID)

	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBodyBytes)

	resolved, ok := h.authenticate(r)
	if !ok {
		h.deny(w, r, start, nil, "", "", requestedModel, http.StatusUnauthorized, errInvalidOrRevokedKey, nil)
		return
	}

	now := time.Now()
	if code, ok := checkUserStatus(resolved, now); !ok {
		h.deny(w, r, start, &resolved, "", "", requestedModel, http.StatusUnauthorized, code, nil)
		return
	}

	access, ok := resolved.Access[requestedModel]
	if !ok {
		h.deny(w, r, start, &resolved, "", "", requestedModel, http.StatusForbidden, errModelNotPermitted, nil)
		return
	}
	if access.ModelBlocked || access.ModelStatus == "retired" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusForbidden, errModelBlocked, nil)
		return
	}

	if status, code, extra := h.policyDenial(now, resolved.UserID, access); code != "" {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, status, code, extra)
		return
	}

	var reservation *reqqueue.Reservation
	if h.queue != nil {
		admitCtx, cancelAdmit := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		res, admitted := h.queue.Acquire(admitCtx, reqqueue.BytesCost(r.ContentLength))
		cancelAdmit()
		if !admitted {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errServerBusy, nil)
			return
		}
		reservation = res
		defer reservation.Release()
	}

	var req proxyRequest
	if err := decodeProxyRequest(r, &req); err != nil {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errInvalidRequest, nil)
		return
	}

	blocks, err := resolveContent(req)
	if err != nil {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errInvalidRequest, map[string]any{"detail": err.Error()})
		return
	}

	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errInvalidRequest, map[string]any{"detail": errInvalidMaxTokens.Error()})
		return
	}

	if unsupported, ok := unsupportedBlockType(access, blocks); !ok {
		h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errContentTypeNotSupported, map[string]any{
			"content_type": unsupported,
		})
		return
	}

	resolvedMaxTokens := defaultMaxTokens
	if req.MaxTokens != nil {
		resolvedMaxTokens = *req.MaxTokens
	}

	bodyTemplate := access.Provider.RequestBodyTemplate
	searchFamily := ""
	if req.WebSearch {
		if !access.SupportsWebSearch || !access.Provider.SupportsWebSearch ||
			len(access.Provider.WebSearchRequestBodyTemplate) == 0 {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusBadRequest, errWebSearchNotSupported, nil)
			return
		}
		bodyTemplate = access.Provider.WebSearchRequestBodyTemplate
		searchFamily = access.Provider.ProviderFamily
	}

	if reservation != nil {
		admitCtx, cancelAdmit := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		admitted := reservation.Grow(admitCtx, reqqueue.RequestCost(blocks))
		cancelAdmit()
		if !admitted {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errServerBusy, nil)
			return
		}
	}

	if limiter := access.Provider.Limiter; limiter != nil {
		limitCtx, cancelLimit := context.WithTimeout(r.Context(), h.queueAdmitTimeout)
		releaseProvider, err := limiter.Acquire(limitCtx)
		cancelLimit()
		if err != nil {
			h.deny(w, r, start, &resolved, access.Provider.ID, access.ModelID, requestedModel, http.StatusServiceUnavailable, errProviderBusy, nil)
			return
		}
		defer releaseProvider()
	}

	statusCode := http.StatusBadGateway
	tokensIn, tokensOut := 0, 0
	var inputCost, outputCost float64
	outcome := "success"

	searchAcc := newSearchAccumulator()

	h.mem.IncrProviderHourly(access.Provider.ID)
	h.mem.IncrUserModelHourly(resolved.UserID, access.ModelID)

	streamCtx, cancelStream := context.WithCancel(r.Context())
	idleTimer := time.AfterFunc(streamIdleTimeout, cancelStream)
	defer idleTimer.Stop()
	defer cancelStream()

	resp, doErr := doUpstreamRequestWithRetry(streamCtx, h.httpClient, access.Provider, bodyTemplate, access.ResolvedModel, blocks, req.MaxTokens)
	if doErr != nil {
		var buildErr *buildRequestError
		if errors.As(doErr, &buildErr) {
			slog.Error("provider request template failed to render",
				"request_id", requestID, "provider", access.Provider.ID, "err", doErr)
			writeError(w, http.StatusInternalServerError, "internal_error", nil)
			return
		}
	}

	if doErr == nil {
		defer resp.Body.Close()
		statusCode = resp.StatusCode

		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
			outcome = "upstream_error"
			slog.Warn("upstream returned an error",
				"request_id", requestID, "provider", access.Provider.ID,
				"model", requestedModel, "provider_status", resp.StatusCode)
			writeError(w, http.StatusBadGateway, "upstream_unavailable", map[string]any{
				"provider_status":  resp.StatusCode,
				"provider_message": extractProviderErrorMessage(errBody),
			})
		} else {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			flusher, canFlush := w.(http.Flusher)

			reader := bufio.NewReaderSize(resp.Body, 64<<10)
			chunkBuf := newChunkWriter(requestID)

			streamErr := streamDeltas(reader,
				streamSpec{
					deltaPathSegs: access.Provider.DeltaPathSegs,
					doneSignal:    access.Provider.ResponseDoneSignal,
					usageInSegs:   access.Provider.UsageInPathSegs,
					usageOutSegs:  access.Provider.UsageOutPathSegs,
					searchFamily:  searchFamily,
				},
				streamCallbacks{
					onDelta: func(delta string) {
						idleTimer.Reset(streamIdleTimeout)
						chunkBuf.writeDelta(w, delta)
						if canFlush {
							flusher.Flush()
						}
					},
					onUsageIn:  func(n int) { tokensIn = n },
					onUsageOut: func(n int) { tokensOut = n },
					onSearch: func(ev providerfamily.SearchEvent) {
						idleTimer.Reset(streamIdleTimeout)

						before := len(searchAcc.citations)
						searchAcc.add(ev)
						if len(searchAcc.citations) > before {
							writeSourcesChunk(w, requestID, searchAcc.citations[before:])
							if canFlush {
								flusher.Flush()
							}
						}
					},
				},
			)
			if streamErr != nil {
				outcome = "upstream_error"
				slog.Warn("upstream stream ended before the provider signalled completion",
					"request_id", requestID, "provider", access.Provider.ID,
					"model", requestedModel, "err", streamErr)
			}
			writeProxyDoneChunk(w, requestID, tokensIn, tokensOut, resolvedMaxTokens, searchAcc.result(), streamErr != nil)
			if canFlush {
				flusher.Flush()
			}
		}
	} else {
		slog.Warn("upstream unreachable",
			"request_id", requestID, "provider", access.Provider.ID,
			"model", requestedModel, "err", doErr)
		writeError(w, http.StatusBadGateway, "upstream_unavailable", nil)
		outcome = "upstream_error"
	}

	inputCost = (float64(tokensIn) / 1000.0) * access.InputPricePer1kTokens
	outputCost = (float64(tokensOut) / 1000.0) * access.OutputPricePer1kTokens

	webSearches := searchAcc.billableSearches()
	inputCost += float64(webSearches) * access.Provider.WebSearchPricePerCall

	h.mem.AddDailySpendUSD(resolved.UserID, access.ModelID, inputCost+outputCost)

	if !h.logsEnabled(access.Provider.ID) {
		return
	}

	logID, err := uuid.NewV7()
	if err != nil {
		return
	}

	var sourceIP *string
	if ip := httputil.ClientIP(r, h.trustProxyHeader); ip != "" {
		sourceIP = &ip
	}
	var userAgent *string
	if ua := r.UserAgent(); ua != "" {
		userAgent = &ua
	}
	keyID, userID, providerID, modelID := resolved.KeyID, resolved.UserID, access.Provider.ID, access.ModelID
	reqModel := requestedModel

	h.telemetry.Enqueue(logEvent{
		ID:             logID,
		MeshKeyID:      &keyID,
		UserID:         &userID,
		ProviderID:     &providerID,
		ModelID:        &modelID,
		RequestedModel: &reqModel,
		Outcome:        outcome,
		SourceIP:       sourceIP,
		UserAgent:      userAgent,
		StatusCode:     statusCode,
		LatencyMs:      int(time.Since(start).Milliseconds()),
		TokensIn:       tokensIn,
		TokensOut:      tokensOut,
		InputCost:      inputCost,
		OutputCost:     outputCost,
		WebSearches:    webSearches,
	})
}

func unsupportedBlockType(access memstate.ModelAccess, blocks []providerfamily.Block) (blockType string, ok bool) {
	for _, b := range blocks {
		switch b.Type {
		case providerfamily.BlockText:
			continue
		case providerfamily.BlockImage:
			if !access.SupportsImageIn {
				return string(b.Type), false
			}
		case providerfamily.BlockDocument:
			if !access.SupportsDocumentIn {
				return string(b.Type), false
			}
		case providerfamily.BlockAudio:
			if !access.SupportsAudioIn {
				return string(b.Type), false
			}
		case providerfamily.BlockVideo:
			if !access.SupportsVideoIn {
				return string(b.Type), false
			}
		}
	}
	return "", true
}

func checkTimeWindow(now time.Time, p memstate.PolicySnapshot) (resumeAt time.Time, ok bool) {
	loc := p.Location
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)

	dayOK := len(p.ActiveDays) == 0
	for _, d := range p.ActiveDays {
		if int(local.Weekday()) == d {
			dayOK = true
			break
		}
	}
	if !dayOK {
		return local.Add(24 * time.Hour), false
	}

	if !p.HasWindow {
		return time.Time{}, true
	}
	fromMinutes, toMinutes := p.FromMinutes, p.ToMinutes
	nowMinutes := local.Hour()*60 + local.Minute()
	if nowMinutes >= fromMinutes && nowMinutes < toMinutes {
		return time.Time{}, true
	}
	resumeToday := time.Date(local.Year(), local.Month(), local.Day(), fromMinutes/60, fromMinutes%60, 0, 0, loc)
	if nowMinutes >= toMinutes {
		resumeToday = resumeToday.Add(24 * time.Hour)
	}
	return resumeToday, false
}
