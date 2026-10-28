// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The brand images (ADR-0116): the admin operations that check, set, and clear the instance's
// icon, logo, and dark logo (manage_settings), and Instance.icon / logo / logo_dark of
// GET /instance.

// ProblemTypeInvalidBrand is the problem type of a brand image that breaks a rule (422), whose
// brand_error says why.
const ProblemTypeInvalidBrand = "/api/v1/problems/invalid-brand"

// setBrandPattern is the pattern of setAdminBrand, whose body carries an SVG and up to six PNG
// renditions.
const setBrandPattern = "PUT " + BasePath + "/admin/instance/brand/{kind}"

// maxSetBrandBody is the largest body of setAdminBrand: the largest SVG, six of the largest
// renditions, and the part headers and the background within uploadOverhead.
const maxSetBrandBody = instance.MaxBrandSVGInputBytes + 6*instance.MaxBrandPNGBytes + uploadOverhead

// maxBrandTextPart bounds the bytes of the text parts (kind, background).
const maxBrandTextPart = 32

// BrandImages runs the use cases of the brand images. *appinstance.Service implements it.
type BrandImages interface {
	// Brands returns the current brand images; an unset kind is nil.
	Brands(ctx context.Context) (instance.Brands, error)
	// SanitizeBrand checks an SVG without saving it; it fails with an *instance.BrandError.
	SanitizeBrand(kind instance.BrandKind, svg []byte) (instance.BrandSVG, error)
	// SetBrand records a new brand revision, audited; it fails with an *instance.BrandError.
	SetBrand(ctx context.Context, in appinstance.SetBrandInput) (appinstance.SetBrandResult, error)
	// ClearBrand brings a kind back to the built-in images, audited.
	ClearBrand(ctx context.Context, kind instance.BrandKind, revisedByUserID string) (instance.BrandRevision, error)
}

// instanceBrandImage returns a brand revision as Instance presents it, or nil without one.
func (s *Server) instanceBrandImage(rev *instance.BrandRevision) *InstanceBrandImage {
	if rev == nil {
		return nil
	}
	svg, ok := rev.File(instance.RenditionSVG)
	if !ok {
		return nil
	}
	out := &InstanceBrandImage{
		Revision: rev.ID, SvgUrl: s.origin() + instance.BrandFilePath(rev.ID, instance.RenditionSVG),
		Width: svg.Width, Height: svg.Height, Renditions: []BrandFile{},
	}
	for _, r := range instance.BrandRenditions(rev.Kind) {
		if f, ok := rev.File(r); ok {
			out.Renditions = append(out.Renditions, BrandFile{
				Rendition: BrandRendition(r), Url: s.origin() + instance.BrandFilePath(rev.ID, r), Width: f.Width, Height: f.Height,
			})
		}
	}
	return out
}

func (s *Server) adminBrandImage(rev *instance.BrandRevision) *AdminBrandImage {
	img := s.instanceBrandImage(rev)
	if img == nil {
		return nil
	}
	out := &AdminBrandImage{
		Revision: img.Revision, SvgUrl: img.SvgUrl, Width: img.Width, Height: img.Height, Renditions: img.Renditions,
		RevisedAt: rev.RevisedAt.UTC(),
	}
	if rev.Background != "" {
		out.Background = &rev.Background
	}
	return out
}

// instanceBrands sets Instance.icon, logo, and logo_dark.
func (s *Server) instanceBrands(ctx context.Context, out *Instance) error {
	if s.o.BrandImages == nil {
		return nil
	}
	b, err := s.o.BrandImages.Brands(ctx)
	if err != nil {
		return fmt.Errorf("clientapi: instance brand images: %w", err)
	}
	out.Icon, out.Logo, out.LogoDark = s.instanceBrandImage(b.Icon), s.instanceBrandImage(b.Logo), s.instanceBrandImage(b.LogoDark)
	return nil
}

func brandDropped(in []instance.BrandDropped) []BrandDropped {
	out := make([]BrandDropped, len(in))
	for i, d := range in {
		out[i] = BrandDropped{Kind: BrandDroppedKind(d.Kind), Name: d.Name}
	}
	return out
}

// brandProblem returns the 422 of a rejected brand image.
func (s *Server) brandProblem(ctx context.Context, be *instance.BrandError) BrandProblemApplicationProblemPlusJSONResponse {
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidBrand, l.ServerErrorsInvalidBrand(), "")
	e := BrandError{Code: BrandErrorCode(be.Code)}
	if be.Rendition != "" {
		r := string(be.Rendition)
		e.Rendition = &r
	}
	if be.Detail != "" {
		e.Detail = &be.Detail
	}
	return BrandProblemApplicationProblemPlusJSONResponse{
		Body:    BrandProblem{Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, BrandError: &e},
		Headers: BrandProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}
}

// requireBrandAdmin returns the problem of a viewer without manage_settings, or of a server
// without brand images.
func (s *Server) requireBrandAdmin(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil || p != nil {
		return p, err
	}
	if s.o.BrandImages == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// GetAdminBrands serves GET /api/v1/admin/instance/brand.
func (s *Server) GetAdminBrands(ctx context.Context, _ GetAdminBrandsRequestObject) (GetAdminBrandsResponseObject, error) {
	if p, err := s.requireBrandAdmin(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return GetAdminBrandsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	b, err := s.o.BrandImages.Brands(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: admin brand images: %w", err)
	}
	cache := cacheNoStore
	return GetAdminBrands200JSONResponse{
		Body:    AdminBrands{Icon: s.adminBrandImage(b.Icon), Logo: s.adminBrandImage(b.Logo), LogoDark: s.adminBrandImage(b.LogoDark)},
		Headers: GetAdminBrands200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// brandParts are the parts of a brand request that the handler reads.
type brandParts struct {
	kind, background string
	svg              []byte
	renditions       map[instance.BrandRendition][]byte
}

// readBrandParts reads the parts of a brand request: kind and background (short text), svg,
// and the PNG renditions by their names with "_" for "-". Other parts are skipped; a part given twice is a 400. An
// SVG or a PNG over its limit is the 422 of the brand error too_large.
func (s *Server) readBrandParts(ctx context.Context, body *multipart.Reader) (brandParts, *problem, *instance.BrandError) {
	l := s.Localizer(ctx)
	out := brandParts{renditions: map[instance.BrandRendition][]byte{}}
	seen := map[string]bool{}
	bad := func(name string) *problem {
		p := s.problem(ctx, http.StatusBadRequest, ProblemTypeBlank, l.ServerErrorsBadRequest(), l.ServerErrorsInvalidParameter(name))
		return &p
	}
	readErr := func(err error, name string) *problem {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			p := s.problem(ctx, http.StatusRequestEntityTooLarge, ProblemTypeBlank, l.ServerErrorsRequestTooLarge(), "")
			return &p
		}
		return bad(name)
	}
	if body == nil {
		return out, bad("body"), nil
	}
	for {
		part, err := body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, readErr(err, "body"), nil
		}
		name := part.FormName()
		var limit int64
		// A rendition's part is named after it with "_" for "-" (favicon_32).
		rendition, isRendition := instance.ParseBrandRendition(strings.ReplaceAll(name, "_", "-"))
		switch {
		case name == "kind" || name == "background":
			limit = maxBrandTextPart
		case name == "svg":
			limit = instance.MaxBrandSVGInputBytes
		case isRendition && rendition != instance.RenditionSVG:
			limit = instance.MaxBrandPNGBytes
		default:
			continue
		}
		if seen[name] {
			return out, bad(name), nil
		}
		seen[name] = true
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			return out, readErr(err, name), nil
		}
		if int64(len(data)) > limit {
			if name == "kind" || name == "background" {
				return out, bad(name), nil
			}
			r := instance.RenditionSVG
			if name != "svg" {
				r = rendition
			}
			return out, nil, &instance.BrandError{Code: instance.BrandTooLarge, Rendition: r, Detail: fmt.Sprintf("the file is over %d bytes", limit)}
		}
		switch name {
		case "kind":
			out.kind = string(data)
		case "background":
			out.background = string(data)
		case "svg":
			out.svg = data
		default:
			out.renditions[rendition] = data
		}
	}
	if !seen["svg"] {
		return out, bad("svg"), nil
	}
	return out, nil, nil
}

// SanitizeAdminBrand serves POST /api/v1/admin/instance/brand/sanitize.
func (s *Server) SanitizeAdminBrand(ctx context.Context, req SanitizeAdminBrandRequestObject) (SanitizeAdminBrandResponseObject, error) {
	if p, err := s.requireBrandAdmin(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return SanitizeAdminBranddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	parts, p, be := s.readBrandParts(ctx, req.Body)
	if p != nil {
		return SanitizeAdminBranddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if be != nil {
		return SanitizeAdminBrand422ApplicationProblemPlusJSONResponse{s.brandProblem(ctx, be)}, nil
	}
	kind, ok := instance.ParseBrandKind(parts.kind)
	if !ok {
		return SanitizeAdminBranddefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	svg, err := s.o.BrandImages.SanitizeBrand(kind, parts.svg)
	if errors.As(err, &be) {
		return SanitizeAdminBrand422ApplicationProblemPlusJSONResponse{s.brandProblem(ctx, be)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: sanitize brand: %w", err)
	}
	out := BrandSanitized{
		Svg: string(svg.SVG), ViewBoxWidth: float32(svg.ViewWidth), ViewBoxHeight: float32(svg.ViewHeight),
		Renditions: []BrandRenditionSize{}, Dropped: brandDropped(svg.Dropped),
	}
	for _, r := range instance.BrandRenditions(kind) {
		w, h := instance.BrandRenditionSize(r, svg.ViewWidth, svg.ViewHeight)
		out.Renditions = append(out.Renditions, BrandRenditionSize{Rendition: BrandRendition(r), Width: w, Height: h})
	}
	cache := cacheNoStore
	return SanitizeAdminBrand200JSONResponse{Body: out, Headers: SanitizeAdminBrand200ResponseHeaders{CacheControl: &cache}}, nil
}

// SetAdminBrand serves PUT /api/v1/admin/instance/brand/{kind}.
func (s *Server) SetAdminBrand(ctx context.Context, req SetAdminBrandRequestObject) (SetAdminBrandResponseObject, error) {
	if p, err := s.requireBrandAdmin(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return SetAdminBranddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	kind, ok := instance.ParseBrandKind(string(req.Kind))
	if !ok {
		return SetAdminBranddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	parts, p, be := s.readBrandParts(ctx, req.Body)
	if p != nil {
		return SetAdminBranddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if be != nil {
		return SetAdminBrand422ApplicationProblemPlusJSONResponse{s.brandProblem(ctx, be)}, nil
	}
	res, err := s.o.BrandImages.SetBrand(ctx, appinstance.SetBrandInput{
		Kind: kind, SVG: parts.svg, Renditions: parts.renditions, Background: parts.background, RevisedByUserID: v.UserID,
	})
	if errors.As(err, &be) {
		return SetAdminBrand422ApplicationProblemPlusJSONResponse{s.brandProblem(ctx, be)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: set brand: %w", err)
	}
	img := s.adminBrandImage(&res.Revision)
	dropped := brandDropped(res.Dropped)
	img.Dropped = &dropped
	cache := cacheNoStore
	return SetAdminBrand200JSONResponse{Body: *img, Headers: SetAdminBrand200ResponseHeaders{CacheControl: &cache}}, nil
}

// ClearAdminBrand serves DELETE /api/v1/admin/instance/brand/{kind}.
func (s *Server) ClearAdminBrand(ctx context.Context, req ClearAdminBrandRequestObject) (ClearAdminBrandResponseObject, error) {
	if p, err := s.requireBrandAdmin(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ClearAdminBranddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	kind, ok := instance.ParseBrandKind(string(req.Kind))
	if !ok {
		return ClearAdminBranddefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if _, err := s.o.BrandImages.ClearBrand(ctx, kind, v.UserID); err != nil {
		return nil, fmt.Errorf("clientapi: clear brand: %w", err)
	}
	return ClearAdminBrand204Response{}, nil
}
