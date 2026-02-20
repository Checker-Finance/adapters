package braza

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/store"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"go.uber.org/zap"
)

type ProductSyncer struct {
	logger   *zap.Logger
	store    store.Store
	baseURL  string
	authMgr  *auth.Manager
	client   *http.Client
	interval time.Duration
}

func NewProductSyncer(logger *zap.Logger, store store.Store, baseURL string, authMgr *auth.Manager, interval time.Duration) *ProductSyncer {
	return &ProductSyncer{
		logger:   logger,
		store:    store,
		baseURL:  baseURL,
		authMgr:  authMgr,
		interval: interval,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *ProductSyncer) Start(ctx context.Context, clientID string, cfg config.Config) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	creds, err := p.authMgr.GetCredentials(ctx, cfg, clientID, cfg.Venue)
	if err != nil {
		p.logger.Error("Failed to get credentials", zap.Error(err))
		return
	}
	for {
		if err := p.syncOnce(ctx, clientID, cfg.Venue, creds); err != nil {
			p.logger.Warn("braza.product_sync_failed", zap.Error(err))
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			p.logger.Info("braza.product_sync_stopped")
			return
		}
	}
}

func (p *ProductSyncer) syncOnce(ctx context.Context, clientID, venue string, creds auth.Credentials) error {
	token, err := p.authMgr.GetValidToken(ctx, clientID, creds)
	if err != nil {
		return fmt.Errorf("token error: %w", err)
	}

	url := fmt.Sprintf("%s/trader-api/product/list", p.baseURL)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("braza product sync failed: %d", resp.StatusCode)
	}

	var data BrazaProductListResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	for _, product := range data.Results {
		//p.logger.Info("product", zap.Any("product", product))
		pr := model.Product{
			VenueCode:        venue,
			ProductID:        parseIntString(product.ID),
			InstrumentSymbol: product.Par,
			ProductName:      product.Nome,
			SecondaryID:      parseIntString(product.IDProductCompany),
			IsBlocked:        false,
			AsOf:             time.Now(),
		}
		if err := p.store.StoreProduct(ctx, pr); err != nil {
			p.logger.Warn("braza.product_upsert_failed", zap.Error(err))
		}
	}

	p.logger.Info("braza.product_sync_complete",
		zap.Int("count", len(data.Results)),
		zap.String("client", clientID),
	)
	return nil
}
