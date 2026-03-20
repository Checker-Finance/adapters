package braza

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

type ProductResolver struct {
	baseURL string
	client  *http.Client

	cache map[string]model.Product
	mu    sync.RWMutex
	ttl   time.Duration
	last  time.Time

	mapper *Mapper
}

func NewProductResolver(baseURL string) *ProductResolver {
	ttl := 1 * time.Minute
	slog.Info("Initializing product resolver with TTL", "ttl", ttl)
	return &ProductResolver{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache:  make(map[string]model.Product),
		ttl:    ttl,
		mapper: NewMapper(),
	}
}

func (r *ProductResolver) IsStale() bool {
	return time.Since(r.last) > r.ttl
}

// ResolveProductID looks up a product ID by instrument (e.g. "USDC/BRL").
func (r *ProductResolver) ResolveProductID(ctx context.Context, instrument string) (int, error) {
	inst := NormalizePairForBraza(instrument)
	inst = strings.ReplaceAll(inst, ":", "")
	p, err := r.resolveBestProduct(inst)

	if err != nil {
		slog.Error("Failed to resolve product", "error", err)
		return 0, err
	}

	slog.Info("found product", "instrument", instrument, "normalizedInstrument", inst, "product", p)
	return strconv.Atoi(p.ProductID)
}

func (r *ProductResolver) resolveBestProduct(pair string) (*model.Product, error) {
	normalized := NormalizePairForBraza(pair)

	var desired []string
	if isAfterCutoff(time.Now()) {
		desired = []string{
			"CRYPTO D1/D1",
			"CRYPTO D2/D2",
		}
	} else {
		desired = []string{
			"CRYPTO D0/D0",
			"CRYPTO D1/D1",
			"CRYPTO D2/D2",
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, name := range desired {
		searchKey := cacheKey(normalized, name)
		if prod, ok := r.cache[searchKey]; ok {
			return &prod, nil
		}
	}

	return nil, fmt.Errorf("no product found for pair %s", pair)
}

func (r *ProductResolver) setProducts(products []BrazaProductDef) {
	tmp := make(map[string]model.Product)
	for _, p := range products {
		key := cacheKey(NormalizePairForBraza(p.Par), p.Nome)
		//slog.Info("setting product", "key", key)
		tmp[key] = r.mapper.FromBrazaProduct(p)
	}

	r.mu.Lock()
	r.cache = tmp
	r.last = time.Now()
	r.mu.Unlock()

	nextSync := time.Now().Add(r.ttl)
	slog.Info("braza.product_sync_next",
		"nextSync", nextSync.Format(time.RFC1123),
	)
}

// 16:00 São Paulo time
const brazaCutoffHour = 16

func isAfterCutoff(now time.Time) bool {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.UTC // fallback
	}

	brNow := now.In(loc)

	cutoff := time.Date(
		brNow.Year(), brNow.Month(), brNow.Day(),
		brazaCutoffHour, 0, 0, 0,
		loc,
	)

	return brNow.After(cutoff)
}

func (r *ProductResolver) ListProducts(venue string) ([]model.Product, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Create a slice with capacity = len(r.cache) but length = 0
	snapshot := make([]model.Product, 0, len(r.cache))
	for _, prod := range r.cache {
		snapshot = append(snapshot, prod)
	}

	return snapshot, nil
}

func cacheKey(par, nome string) string {
	return strings.ToUpper(par) + "|" + strings.ToUpper(nome)
}
