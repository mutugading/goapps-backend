package mbsourceautofill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

// Link sources reported by Explain.
const (
	LinkFgMatch   = "FG_MATCH"
	LinkShadeOnly = "SHADE_ONLY"
	LinkSuperba   = "SUPERBA"
	LinkNone      = "NONE"

	mbTypeCode = "MB"
)

var (
	// ErrLinkProductNotFound is returned when the product does not exist.
	ErrLinkProductNotFound = errors.New("cost product not found")
	// ErrLinkMBType is returned by Relink for MB-typed products (they never get an MB source).
	ErrLinkMBType = errors.New("MB-type product has no MB source link")
	// ErrLinkNotConfigured is returned when the link service lacks a dependency.
	ErrLinkNotConfigured = errors.New("mb link service not configured")
)

// LinkProduct is the product info needed to explain / re-link its MB source.
type LinkProduct struct {
	ProductSysID  int64
	ProductCode   string
	TypeCode      string
	Shade         string
	Locked        bool
	CurrentSPCode string
}

// LinkReader is the read port for the link explanation.
type LinkReader interface {
	// GetLinkProduct loads one product; ErrLinkProductNotFound when absent.
	GetLinkProduct(ctx context.Context, productSysID int64) (LinkProduct, error)
	// ProductCodes returns product code by id for the given ids (one batched query).
	ProductCodes(ctx context.Context, ids []int64) (map[int64]string, error)
}

// ForceApplier overwrites the MB source group of one product (MBSourceAutoFillStore.ApplyForce).
type ForceApplier interface {
	ApplyForce(ctx context.Context, ps ParamSet, plan Plan, actor string) error
}

// FgExplainer explains the FG lookup of one product (*mbsource.MBSpinFgProvider).
type FgExplainer interface {
	Explain(ctx context.Context, t mbsource.Target) (mbsource.FgExplain, error)
}

// Link is the explanation of how a product's MB source is (or would be) resolved.
type Link struct {
	LinkSource        string
	SpinID            string
	SpinCode          string
	SpinShadeCode     string
	FgProductID       int64
	FgProductCode     string
	CandidatePoyIDs   []int64
	Reason            string
	CurrentParamValue string
	ParamMatches      bool
}

// LinkService serves GetCostProductMbLink (read-only) and RelinkCostProductMb.
type LinkService struct {
	reader   LinkReader
	resolver TargetResolver
	fg       FgExplainer
	applier  ForceApplier
	svc      *Service
}

// NewLinkService constructs the link service. svc supplies paramSet(); fg may be nil (no FG info).
func NewLinkService(reader LinkReader, resolver TargetResolver, fg FgExplainer, applier ForceApplier, svc *Service) *LinkService {
	return &LinkService{reader: reader, resolver: resolver, fg: fg, applier: applier, svc: svc}
}

// Explain resolves the product read-only. MB type / no shade yield NONE with a reason.
func (l *LinkService) Explain(ctx context.Context, productSysID int64) (Link, error) {
	link, _, _, err := l.explain(ctx, productSysID)
	return link, err
}

// explain returns the link, the product and the resolution it is based on (nil when NONE).
func (l *LinkService) explain(ctx context.Context, pid int64) (Link, LinkProduct, *mbsource.Resolution, error) {
	if l.reader == nil || l.resolver == nil {
		return Link{}, LinkProduct{}, nil, ErrLinkNotConfigured
	}
	p, err := l.reader.GetLinkProduct(ctx, pid)
	if err != nil {
		return Link{}, LinkProduct{}, nil, err
	}
	link := Link{LinkSource: LinkNone, CurrentParamValue: p.CurrentSPCode}
	if strings.EqualFold(strings.TrimSpace(p.TypeCode), mbTypeCode) {
		link.Reason = "produk tipe MB"
		return link, p, nil, nil
	}
	shade := mbsource.NormalizeShade(p.Shade)
	if shade == "" {
		link.Reason = "produk tanpa shade"
		return link, p, nil, nil
	}
	res, found, err := l.lookup(ctx, &link, mbsource.Target{ProductSysID: pid, Shade: shade})
	if err != nil {
		return Link{}, p, nil, err
	}
	if !found {
		link.Reason = "tidak ada spin atau superba untuk shade " + shade
		return link, p, nil, nil
	}
	if err := l.fill(ctx, &link, res, shade); err != nil {
		return Link{}, p, nil, err
	}
	link.ParamMatches = mbsource.NormalizeShade(p.CurrentSPCode) == mbsource.NormalizeShade(res.SPCode)
	return link, p, &res, nil
}

// lookup resolves one target once: an FG hit is used directly (it is the first provider
// anyway); otherwise the resolver chain runs. Candidates are recorded on link.
func (l *LinkService) lookup(ctx context.Context, link *Link, t mbsource.Target) (mbsource.Resolution, bool, error) {
	if l.fg != nil {
		ex, err := l.fg.Explain(ctx, t)
		if err != nil {
			return mbsource.Resolution{}, false, fmt.Errorf("explain fg link: %w", err)
		}
		link.CandidatePoyIDs = ex.Candidates
		if ex.Hit != nil {
			return *ex.Hit, true, nil
		}
	}
	got, err := l.resolver.ResolveTargets(ctx, []mbsource.Target{t})
	if err != nil {
		return mbsource.Resolution{}, false, fmt.Errorf("resolve mb link: %w", err)
	}
	res, found := got[t.ProductSysID]
	return res, found, nil
}

func (l *LinkService) fill(ctx context.Context, link *Link, res mbsource.Resolution, shade string) error {
	link.SpinCode = res.SPCode
	link.SpinShadeCode = shade
	if res.SpinID != nil {
		link.SpinID = res.SpinID.String()
	}
	switch res.Source {
	case mbsource.SourceMBSpinFg:
		link.LinkSource = LinkFgMatch
		if res.FgProductID != nil {
			link.FgProductID = *res.FgProductID
			codes, err := l.reader.ProductCodes(ctx, []int64{*res.FgProductID})
			if err != nil {
				return fmt.Errorf("fg product code: %w", err)
			}
			link.FgProductCode = codes[*res.FgProductID]
		}
		link.Reason = "spin FG untuk POY " + link.FgProductCode
	case mbsource.SourceSuperbaCostSP:
		link.LinkSource = LinkSuperba
		link.Reason = "tidak ada spin, fallback superba"
	default:
		link.LinkSource = LinkShadeOnly
		link.Reason = "tidak ada spin ber-FG, fallback shade"
	}
	return nil
}

// Relink force-writes the resolved MB source of one product, then returns the new link.
// MB type is rejected; NONE writes nothing; a locked product surfaces the applier error.
func (l *LinkService) Relink(ctx context.Context, productSysID int64, actor string) (Link, error) {
	link, p, res, err := l.explain(ctx, productSysID)
	if err != nil {
		return Link{}, err
	}
	if strings.EqualFold(strings.TrimSpace(p.TypeCode), mbTypeCode) {
		return Link{}, ErrLinkMBType
	}
	if link.LinkSource == LinkNone || res == nil {
		return link, nil
	}
	if l.applier == nil || l.svc == nil {
		return Link{}, ErrLinkNotConfigured
	}
	ps, err := l.svc.paramSet(ctx)
	if err != nil {
		return Link{}, err
	}
	if err := l.applier.ApplyForce(ctx, ps, Plan{ProductSysID: productSysID, Resolution: *res}, actor); err != nil {
		return Link{}, err
	}
	link.CurrentParamValue = res.SPCode
	link.ParamMatches = true
	return link, nil
}
