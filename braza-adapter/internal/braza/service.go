package braza

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	intsecrets "github.com/Checker-Finance/adapters/braza-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Service orchestrates Braza API polling, quote/trade submission,
// and normalized event publishing to NATS.
type Service struct {
	ctx             context.Context
	cfg             config.Config
	logger          *zap.Logger
	nc              *nats.Conn
	baseURL         string
	authMgr         *auth.Manager
	resolver        *intsecrets.AWSResolver
	rateMgr         *rate.Manager
	publisher       *publisher.Publisher
	store           store.Store
	mapper          *Mapper
	client          *http.Client
	productResolver *ProductResolver
	tradeSyncWriter *legacy.TradeSyncWriter

	poller *Poller
}

// NewService constructs a fully wired Braza adapter service.
func NewService(
	ctx context.Context,
	cfg config.Config,
	logger *zap.Logger,
	nc *nats.Conn,
	baseURL string,
	authMgr *auth.Manager,
	resolver *intsecrets.AWSResolver,
	rateMgr *rate.Manager,
	pub *publisher.Publisher,
	st store.Store,
	productResolver *ProductResolver,
	tradeSyncWriter *legacy.TradeSyncWriter,
) *Service {
	return &Service{
		ctx:       ctx,
		cfg:       cfg,
		logger:    logger,
		nc:        nc,
		baseURL:   baseURL,
		authMgr:   authMgr,
		resolver:  resolver,
		rateMgr:   rateMgr,
		publisher: pub,
		store:     st,
		mapper:    NewMapper(),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		productResolver: productResolver,
		tradeSyncWriter: tradeSyncWriter,
	}
}

func (s *Service) SetPoller(p *Poller) {
	s.poller = p
}

// FetchAndPublishBalances queries Braza balances and persists + publishes events.
func (s *Service) FetchAndPublishBalances(
	ctx context.Context,
	clientID string,
	pub *publisher.Publisher,
	st store.Store,
	creds auth.Credentials,
) error {
	//s.logger.Info("braza.fetch_balances.start",
	//	zap.String("client", clientID),
	//)

	token, err := s.authMgr.GetValidToken(ctx, clientID, creds)
	if err != nil {
		return fmt.Errorf("token_error: %w", err)
	}

	url := fmt.Sprintf("%s/trader-api/me/balance", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("braza balances failed: %d", resp.StatusCode)
	}

	//body, err := io.ReadAll(resp.Body)
	//if err != nil {
	//	return fmt.Errorf("read_body_error: %w", err)
	//}
	//
	//resp.Body = io.NopCloser(bytes.NewBuffer(body))
	//
	//s.logger.Sugar().Infow("braza.balance_response.raw",
	//	"status", resp.StatusCode,
	//	"body", string(body),
	//)

	var balancesResp BrazaBalancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&balancesResp); err != nil {
		return fmt.Errorf("decode_error: %w", err)
	}

	balances := s.mapper.FromBrazaBalances(balancesResp, clientID)
	for _, bal := range balances {
		bal.ClientID = clientID
		bal.Venue = "braza"
		bal.LastUpdated = time.Now().UTC()

		if err := st.RecordBalanceEvent(ctx, bal); err != nil {
			s.logger.Warn("store.record_event_failed",
				zap.String("instrument", bal.Instrument),
				zap.Error(err))
		}

		if err := st.UpdateBalanceSnapshot(ctx, bal); err != nil {
			s.logger.Warn("store.update_snapshot_failed",
				zap.String("instrument", bal.Instrument),
				zap.Error(err))
		}

		if err := pub.Publish(ctx, "evt.balance.update.v1", bal); err != nil {
			s.logger.Warn("publish_failed",
				zap.String("instrument", bal.Instrument),
				zap.Error(err))
		}
	}

	//s.logger.Info("braza.fetch_balances.done",
	//	zap.Int("count", len(balances)),
	//	zap.String("client", clientID))

	return nil
}

// CreateRFQ creates a new RFQ (preview quotation) on Braza.
func (s *Service) CreateRFQ(
	ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	credsMap, err := s.resolver.Resolve(ctx, req.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve creds: %w", err)
	}
	creds := auth.Credentials{
		Username: credsMap.Username,
		Password: credsMap.Password,
	}

	token, err := s.authMgr.GetValidToken(ctx, req.ClientID, creds)
	if err != nil {
		return nil, fmt.Errorf("auth_failed: %w", err)
	}

	brazaReq := s.mapper.ToBrazaRFQ(req)
	if s.productResolver.IsStale() {
		_ = s.syncOnce(ctx, req.ClientID, "BRAZA", creds)
	}

	brazaReq.ProductID, err = s.productResolver.ResolveProductID(ctx, req.CurrencyPair)
	if err != nil {
		return nil, fmt.Errorf("product_resolver.ResolveProductID_failed: %w", err)
	}

	body, _ := json.Marshal(brazaReq)
	s.logger.Info("sending braza RFQ body",
		zap.String("json", pretty(brazaReq)))

	url := fmt.Sprintf("%s/rates-ttl/v2/order/preview-quotation", s.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			s.logger.Error("braza.request_timeout",
				zap.String("client_id", req.ClientID),
				zap.String("url", url),
				zap.Duration("timeout", s.client.Timeout),
				zap.Error(err),
			)
			return nil, fmt.Errorf("timeout: Braza did not respond within %s", s.client.Timeout)
		}

		s.logger.Error("braza.request_failed",
			zap.String("client_id", req.ClientID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("request_failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// --- Handle error responses gracefully ---
	if resp.StatusCode != http.StatusCreated {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)

		// Extract "detail" if present
		detail := ""
		if msg, ok := errBody["detail"].(string); ok {
			detail = msg
		}

		s.logger.Info("braza.rfq_create_failed",
			zap.String("tenant", req.TenantID),
			zap.String("client", req.ClientID),
			zap.Int("status", resp.StatusCode),
			zap.String("reason", detail),
			zap.Any("response", errBody),
		)

		return nil, fmt.Errorf("braza rfq create failed [%d]: %s", resp.StatusCode, detail)
	}

	// --- Decode success response ---
	var quoteResp BrazaQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&quoteResp); err != nil {
		return nil, fmt.Errorf("decode_failed: %w", err)
	}

	quote := s.mapper.FromBrazaQuote(quoteResp, req.ClientID)
	s.logger.Info("braza.rfq_created",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quote.ID),
		zap.Float64("price", quote.Price),
		zap.String("instrument", quote.Instrument),
	)

	return &quote, nil
}

func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*BrazaExecuteResponse, error) {
	s.logger.Info("braza.execute_rfq.start",
		zap.String("client", clientID),
		zap.String("quoteID", quoteID),
	)

	credsMap, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve creds: %w", err)
	}
	creds := auth.Credentials{
		Username: credsMap.Username,
		Password: credsMap.Password,
	}

	token, err := s.authMgr.GetValidToken(ctx, clientID, creds)
	if err != nil {
		return nil, fmt.Errorf("token_error: %w", err)
	}

	url := fmt.Sprintf("%s/rates-ttl/v2/order/%s/execute-order", s.baseURL, quoteID)
	s.logger.Info("braza.rfq_request_sent", zap.String("url", url), zap.String("client", clientID), zap.String("quoteID", quoteID))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request_failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusCreated {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		detail := ""
		if msg, ok := errBody["detail"].(string); ok {
			detail = msg
		}

		s.logger.Warn("braza.execute_rfq",
			zap.String("client", clientID),
			zap.Int("status", resp.StatusCode),
			zap.String("reason", detail),
			zap.Any("response", errBody),
		)

		return nil, fmt.Errorf("braza rfq execution failed [%d]: %s", resp.StatusCode, detail)
	}

	var execResp BrazaExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	status := NormalizeOrderStatus(execResp.StatusOrder)
	execResp.StatusOrder = status
	if strings.ToUpper(status) == "PROCESSING" && s.poller != nil {
		pollCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		orderID, err := s.ResolveOrderIDFromQuote(pollCtx, quoteID)
		if err != nil {
			s.logger.Warn("braza.order_id_resolution_failed",
				zap.String("quoteID", quoteID),
				zap.String("client", clientID),
				zap.Error(err),
			)
		} else {
			go s.poller.PollTradeStatus(ctx, clientID, quoteID, orderID, creds)
		}
	}

	return &execResp, nil
}

// FetchTradeStatus retrieves the latest order/trade status from Braza.
func (s *Service) FetchTradeStatus(
	ctx context.Context,
	clientID,
	orderID string,
	creds auth.Credentials,
) (*BrazaOrderStatus, error) {
	token, err := s.authMgr.GetValidToken(ctx, clientID, creds)
	if err != nil {
		s.logger.Warn("braza.token_resolve_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return nil, err
	}

	url := fmt.Sprintf("%s/trader-api/order/%s", s.baseURL, orderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("braza returned %d for order %s", resp.StatusCode, orderID)
	}

	var statusResp BrazaOrderStatus
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	statusResp.Status = NormalizeOrderStatus(statusResp.Status)
	return &statusResp, nil
}

func (s *Service) ListProducts(ctx context.Context, clientID, venue string) ([]model.Product, error) {
	if s.productResolver.IsStale() {
		credsMap, err := s.resolver.Resolve(ctx, clientID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve creds: %w", err)
		}
		creds := auth.Credentials{
			Username: credsMap.Username,
			Password: credsMap.Password,
		}

		_ = s.syncOnce(ctx, clientID, venue, creds)
	}
	return s.productResolver.ListProducts(venue)
}

// BuildTradeConfirmationFromOrder converts a Braza order status response
// into a canonical TradeConfirmation struct used by TradeSyncWriter.
func (s *Service) BuildTradeConfirmationFromOrder(
	clientID,
	orderID string,
	order *BrazaOrderStatus,
) *model.TradeConfirmation {
	if order == nil {
		return nil
	}

	// Normalize status (Processing, Completed, Failed...)
	normalized := NormalizeOrderStatus(order.Status)

	// Map Braza’s buy/sell direction → canonical "BUY" or "SELL"
	side := strings.ToUpper(order.Side)
	if side == "" {
		side = "UNKNOWN"
	}

	// Braza gives pair as "USDT:BRL" → normalize to "usdt/brl"
	pair := NormalizePairForBraza(order.Instrument)
	raw, _ := json.Marshal(order)
	// Construct the trade confirmation

	s.logger.Info("braza.trade_confirmation_from_order", zap.String("order", string(raw)))
	return &model.TradeConfirmation{
		TradeID:         orderID,
		ClientID:        clientID,
		Venue:           "BRAZA",
		Instrument:      pair,
		Side:            side,
		Quantity:        order.Qty,
		Price:           order.ExecutionPrice,
		ProviderOrderID: strconv.Itoa(order.ID),
		Status:          normalized,       // COMPLETED / FAILED / CANCELED
		ExecutedAt:      time.Now().UTC(), // or order.Timestamp if available
		RawPayload:      string(raw),
	}
}

// ResolveOrderIDFromQuote maps quoteID → rfqID → provider orderID.
func (s *Service) ResolveOrderIDFromQuote(ctx context.Context, quoteID string) (string, error) {
	// Step 1: Find quote record
	quote, err := s.store.GetQuoteByQuoteID(ctx, quoteID)
	if err != nil {
		return "", fmt.Errorf("quote_lookup_failed: %w", err)
	}
	if quote == nil {
		return "", fmt.Errorf("quote_not_found: %s", quoteID)
	}

	rfqID := quote.RFQID
	if rfqID == "" {
		return "", fmt.Errorf("missing_rfq_id_for_quote %s", quoteID)
	}

	// Step 2: Find orderID via RFQ
	orderID, err := s.store.GetOrderIDByRFQ(ctx, rfqID)
	if err != nil {
		return "", fmt.Errorf("order_lookup_failed: %w", err)
	}

	if orderID == "" {
		return "", fmt.Errorf("internal_order_id_missing")
	}

	return orderID, nil
}

// Expose config
func (s *Service) Config() config.Config {
	return s.cfg
}

// Expose resolver
func (s *Service) Resolver() *intsecrets.AWSResolver {
	return s.resolver
}

// Standardize how we build credentials for Braza
func (s *Service) BuildCredentials(r secrets.Credentials) auth.Credentials {
	return auth.Credentials{
		Username: r.Username,
		Password: r.Password,
	}
}

func (s *Service) syncOnce(ctx context.Context, clientID, venue string, creds auth.Credentials) error {
	token, err := s.authMgr.GetValidToken(ctx, clientID, creds)
	if err != nil {
		return fmt.Errorf("token error: %w", err)
	}

	url := fmt.Sprintf("%s/trader-api/product/list", s.baseURL)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("braza product sync failed: %d", resp.StatusCode)
	}

	var data BrazaProductListResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	products := append(make([]BrazaProductDef, 0, len(data.Results)), data.Results...)

	s.productResolver.setProducts(products)

	s.logger.Info("braza.product_sync_complete",
		zap.Int("count", len(data.Results)),
		zap.String("client", clientID),
	)
	return nil
}

// PublishErrorEvent logs and publishes an error event for a failed command.
func (s *Service) PublishErrorEvent(env model.Envelope, err error, code string, logError bool) {
	if logError {
		s.logger.Error("service error event",
			zap.String("code", code),
			zap.String("tenant_id", env.TenantID),
			zap.String("client_id", env.ClientID),
			zap.Error(err),
		)
	}
}

// HandleQuoteRequest processes a NATS quote request command.
func (s *Service) HandleQuoteRequest(ctx context.Context, env model.Envelope, req model.QuoteRequest) error {
	s.logger.Info("HandleQuoteRequest",
		zap.String("tenant_id", env.TenantID),
		zap.String("client_id", env.ClientID),
		zap.String("instrument", req.Instrument),
	)
	return nil
}

// HandleTradeExecute processes a NATS trade execution command.
func (s *Service) HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error {
	s.logger.Info("HandleTradeExecute",
		zap.String("tenant_id", env.TenantID),
		zap.String("client_id", env.ClientID),
		zap.String("quote_id", cmd.QuoteID),
	)
	return nil
}

func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
