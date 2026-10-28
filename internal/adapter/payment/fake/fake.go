// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is a supporters.PaymentProvider that keeps the provider's side in memory, for
// tests (ADR-0074 §1, §6). Its tests run testkit.PaymentProviderContract on it; the tests of the
// supporters module drive the provider's side through its methods (PutSubscription, PutMoney,
// PutChange, Fail) and send it webhook deliveries signed by Deliver.
//
// It is never part of a released binary: only a test build with the build tag e2e_fakes links it
// into the server, as payments_provider = fake (NewPages), which serves a checkout page
// where the buyer pays, fails, or cancels without any money moving (ADR-0117 §2). Such a build
// refuses to start outside profile = development, and the checks of the build tags, the
// Dockerfile, and the release keep the tag out of the released binaries and images.
//
// Its webhooks look like the provider's: a JSON body with id, type, livemode, created, and
// data.object.{id, object}, and a header Fake-Signature: t=<unix>,v1=<hex>[,v1=...] whose v1 is
// the HMAC-SHA256 of "<t>.<body>" with the secret.
package fake

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/supporters"
)

// SignatureHeader is the header of the webhook signatures.
const SignatureHeader = "Fake-Signature"

// The hosts of the URLs that the fake returns.
const (
	CheckoutHost = "checkout.payments.test"
	PortalHost   = "portal.payments.test"
)

// DefaultSecret is the webhook secret of a Provider without one.
const DefaultSecret = "whsec_fake"

// DefaultChangesWindow is how far back ListChanges reads by default, as Stripe's events.
const DefaultChangesWindow = 30 * 24 * time.Hour

// Options configure a Provider.
type Options struct {
	// Name is the provider's name; the default is "fake".
	Name string
	// Mode defaults to supporters.ModeTest.
	Mode supporters.Mode
	// Secret signs the webhooks; the default is DefaultSecret.
	Secret string
	// Location is the time zone of the reports; the default is UTC.
	Location *time.Location
	// Now is the provider's clock; the default is time.Now.
	Now func() time.Time
	// PageSize is the size of the pages of the lists; the default is supporters.PageSize.
	PageSize int
	// ChangesWindow defaults to DefaultChangesWindow.
	ChangesWindow time.Duration
	// Pages makes StartCheckout and PortalURL return the URLs of the provider's own pages, which
	// PageProvider serves, on the origin of the success and return URLs and with refs that cannot
	// be guessed; NewPages sets it. Without it, the URLs are on CheckoutHost and PortalHost.
	Pages bool
}

// Provider is the fake supporters.PaymentProvider. It is safe for concurrent use.
type Provider struct {
	o Options

	mu           sync.Mutex
	seq          int
	requests     int
	failure      error
	products     map[string]supporters.ProductRef // by plan ID
	prices       []*price
	checkouts    map[supporters.CheckoutRef]*checkout
	checkoutKeys map[string]supporters.CheckoutRef
	subs         []supporters.SubscriptionSnapshot
	money        []booked
	changes      []supporters.Notice
	planNames    map[string]string // by plan ID
	portals      map[string]*portal
}

type price struct {
	ref      supporters.PriceRef
	planID   string
	spec     supporters.PriceSpec
	archived bool
}

type checkout struct {
	result    supporters.CheckoutResult
	expiresAt time.Time
	// req is the request of a checkout that StartCheckout made, which its page shows.
	req *supporters.CheckoutRequest
}

// booked is a money movement with what its webhooks name and the order in which it became final.
type booked struct {
	m      supporters.MoneyMovement
	object string
	seq    int
}

var _ supporters.PaymentProvider = (*Provider)(nil)

// New returns a Provider with nothing on its side.
func New(o Options) *Provider {
	if o.Name == "" {
		o.Name = "fake"
	}
	if o.Mode == "" {
		o.Mode = supporters.ModeTest
	}
	if o.Secret == "" {
		o.Secret = DefaultSecret
	}
	if o.Location == nil {
		o.Location = time.UTC
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.PageSize <= 0 {
		o.PageSize = supporters.PageSize
	}
	if o.ChangesWindow <= 0 {
		o.ChangesWindow = DefaultChangesWindow
	}
	return &Provider{
		o: o, products: map[string]supporters.ProductRef{}, checkouts: map[supporters.CheckoutRef]*checkout{},
		checkoutKeys: map[string]supporters.CheckoutRef{}, planNames: map[string]string{}, portals: map[string]*portal{},
	}
}

// Name implements supporters.PaymentProvider.
func (p *Provider) Name() string { return p.o.Name }

// Location implements supporters.PaymentProvider.
func (p *Provider) Location() *time.Location { return p.o.Location }

// Mode implements supporters.PaymentProvider.
func (p *Provider) Mode() supporters.Mode { return p.o.Mode }

// minimums are the smallest recurring charges, in minor units.
var minimums = map[string]int64{"JPY": 50, "USD": 50, "EUR": 50}

// MinimumAmount implements supporters.PaymentProvider.
func (p *Provider) MinimumAmount(currency string) (int64, bool) {
	n, ok := minimums[currency]
	return n, ok
}

// The side of the provider, which tests drive.

// PutSubscription creates or replaces a subscription, as the provider's own changes do.
func (p *Provider) PutSubscription(s supporters.SubscriptionSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.subs {
		if p.subs[i].Ref == s.Ref {
			p.subs[i] = s
			return
		}
	}
	p.subs = append(p.subs, s)
}

// Subscription returns the provider's state of a subscription, without counting a request.
func (p *Provider) Subscription(ref supporters.SubscriptionRef) (supporters.SubscriptionSnapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.subIndex(ref)
	if i < 0 {
		return supporters.SubscriptionSnapshot{}, false
	}
	return p.subs[i], true
}

// PutCheckout creates or replaces a checkout, as a buyer completing it or its expiry does. A
// checkout created so expires an hour after now.
func (p *Provider) PutCheckout(res supporters.CheckoutResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.checkouts[res.Ref]; ok {
		c.result = res
		return
	}
	p.checkouts[res.Ref] = &checkout{result: res, expiresAt: p.o.Now().Add(time.Hour)}
}

// PutMoney books a money movement, or replaces the one of its ID. A movement that is not Final
// is pending: the port does not return it until it is put again as Final. OccurredOn and
// ParentGross are filled in from OccurredAt and the parent. Its webhooks name its ID.
func (p *Provider) PutMoney(m supporters.MoneyMovement) { p.PutMoneyObject(m.ID, m) }

// PutMoneyObject is PutMoney with the object that the movement's webhooks name, such as the
// charge of a payment.
func (p *Provider) PutMoneyObject(object string, m supporters.MoneyMovement) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.putMoney(object, m)
}

// putMoney is PutMoneyObject with p.mu held.
func (p *Provider) putMoney(object string, m supporters.MoneyMovement) {
	m.OccurredOn = supporters.DateOf(m.OccurredAt.In(p.o.Location))
	p.seq++
	for i := range p.money {
		if p.money[i].m.ID == m.ID {
			seq := p.money[i].seq
			if m.Final && !p.money[i].m.Final {
				seq = p.seq // it is booked when it becomes final
			}
			p.money[i] = booked{m: m, object: object, seq: seq}
			return
		}
	}
	p.money = append(p.money, booked{m: m, object: object, seq: p.seq})
}

// PutChange records an event that ListChanges returns.
func (p *Provider) PutChange(n supporters.Notice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.changes = append(p.changes, n)
}

// Fail makes every request fail with err (wrapping one of the errors of the port); nil ends it.
func (p *Provider) Fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failure = err
}

// Requests returns how many requests the port's methods sent so far.
func (p *Provider) Requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

// Deliver returns the header and body of a webhook delivery of n signed at at, with one
// signature per key: "" signs with the secret, any other key with itself. The body's type is
// n.Type, or a type of n.Kind without one; live sets livemode.
func (p *Provider) Deliver(n supporters.Notice, live bool, at time.Time, keys ...string) (http.Header, []byte) {
	typ := n.Type
	if typ == "" {
		typ = map[supporters.NoticeKind]string{
			supporters.NoticeSubscription: "subscription.updated", supporters.NoticeCheckout: "checkout.completed",
			supporters.NoticeMoney: "money.booked",
		}[n.Kind]
		if typ == "" {
			typ = "customer.updated"
		}
	}
	object := map[supporters.NoticeKind]string{
		supporters.NoticeSubscription: "subscription", supporters.NoticeCheckout: "checkout", supporters.NoticeMoney: "money",
	}[n.Kind]
	created := n.OccurredAt
	if created.IsZero() {
		created = at
	}
	body, err := json.Marshal(event{
		ID: n.EventID, Type: typ, Livemode: live, Created: created.Unix(),
		Data: eventData{Object: eventObject{ID: n.ObjectID, Object: object, Subscription: string(n.Subscription)}},
	})
	if err != nil {
		panic(err) // strings and numbers always marshal
	}
	if len(keys) == 0 {
		keys = []string{""}
	}
	t := strconv.FormatInt(at.Unix(), 10)
	parts := []string{"t=" + t}
	for _, k := range keys {
		if k == "" {
			k = p.o.Secret
		}
		parts = append(parts, "v1="+sign(k, t, body))
	}
	h := http.Header{}
	h.Set(SignatureHeader, strings.Join(parts, ","))
	h.Set("Content-Type", "application/json")
	return h, body
}

type event struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Livemode bool      `json:"livemode"`
	Created  int64     `json:"created"`
	Data     eventData `json:"data"`
}

type eventData struct {
	Object eventObject `json:"object"`
}

type eventObject struct {
	ID           string `json:"id"`
	Object       string `json:"object"`
	Subscription string `json:"subscription,omitempty"`
}

func sign(secret, t string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ParseWebhook implements supporters.PaymentProvider.
func (p *Provider) ParseWebhook(header http.Header, body []byte, now time.Time) (supporters.Notice, error) {
	var t string
	var sigs []string
	for part := range strings.SplitSeq(header.Get(SignatureHeader), ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			t = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	unix, err := strconv.ParseInt(t, 10, 64)
	if err != nil || len(sigs) == 0 {
		return supporters.Notice{}, fmt.Errorf("%w: no signature", supporters.ErrInvalidSignature)
	}
	want := []byte(sign(p.o.Secret, t, body))
	if !slices.ContainsFunc(sigs, func(s string) bool { return hmac.Equal([]byte(s), want) }) {
		return supporters.Notice{}, supporters.ErrInvalidSignature
	}
	if d := now.Sub(time.Unix(unix, 0)); d > supporters.WebhookTolerance || d < -supporters.WebhookTolerance {
		return supporters.Notice{}, fmt.Errorf("%w: signed %s from now", supporters.ErrStaleSignature, d)
	}
	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		return supporters.Notice{}, fmt.Errorf("%w: %w", supporters.ErrInvalidPaymentResponse, err)
	}
	if live := p.o.Mode == supporters.ModeLive; ev.Livemode != live {
		return supporters.Notice{}, supporters.ErrModeMismatch
	}
	kind := supporters.NoticeIgnored
	switch {
	case strings.HasPrefix(ev.Type, "subscription."):
		kind = supporters.NoticeSubscription
	case strings.HasPrefix(ev.Type, "checkout."):
		kind = supporters.NoticeCheckout
	case strings.HasPrefix(ev.Type, "money."):
		kind = supporters.NoticeMoney
	}
	return supporters.Notice{
		EventID: ev.ID, Type: ev.Type, Kind: kind, ObjectID: ev.Data.Object.ID,
		Subscription: supporters.SubscriptionRef(ev.Data.Object.Subscription), OccurredAt: time.Unix(ev.Created, 0).UTC(),
	}, nil
}

// request counts a request of the port and returns its failure: the one of Fail, or the end of ctx.
func (p *Provider) request(ctx context.Context) error {
	p.requests++
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", supporters.ErrPaymentUnavailable, err)
	}
	return p.failure
}

func (p *Provider) newRef(prefix string) string {
	p.seq++
	return fmt.Sprintf("%s_%06d", prefix, p.seq)
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

// EnsurePlan implements supporters.PaymentProvider.
func (p *Provider) EnsurePlan(ctx context.Context, spec supporters.PlanSpec) (supporters.PlanRefs, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.PlanRefs{}, err
	}
	if spec.PlanID == "" || spec.Name == "" {
		return supporters.PlanRefs{}, fmt.Errorf("%w: a plan needs an ID and a name", supporters.ErrPaymentInvalidRequest)
	}
	for _, s := range spec.Prices {
		minimum, ok := minimums[s.Currency]
		if !currencyCode.MatchString(s.Currency) || !ok || s.AmountMinor < minimum ||
			(s.Interval != supporters.IntervalMonth && s.Interval != supporters.IntervalYear) {
			return supporters.PlanRefs{}, fmt.Errorf("%w: price %+v", supporters.ErrPaymentInvalidRequest, s)
		}
	}
	p.planNames[spec.PlanID] = spec.Name
	product, ok := p.products[spec.PlanID]
	if !ok {
		product = supporters.ProductRef(p.newRef("prod"))
		p.products[spec.PlanID] = product
	}
	out := supporters.PlanRefs{Product: product}
	keep := map[supporters.PriceRef]bool{}
	for _, s := range spec.Prices {
		i := slices.IndexFunc(p.prices, func(pr *price) bool { return pr.planID == spec.PlanID && !pr.archived && pr.spec == s })
		if i < 0 {
			p.prices = append(p.prices, &price{ref: supporters.PriceRef(p.newRef("price")), planID: spec.PlanID, spec: s})
			i = len(p.prices) - 1
		}
		keep[p.prices[i].ref] = true
		out.Prices = append(out.Prices, supporters.PlanPrice{Spec: s, Ref: p.prices[i].ref})
	}
	for _, pr := range p.prices {
		if pr.planID == spec.PlanID && !pr.archived && !keep[pr.ref] {
			pr.archived = true
			out.Archived = append(out.Archived, pr.ref)
		}
	}
	return out, nil
}

func (p *Provider) activePrice(ref supporters.PriceRef) bool {
	return slices.ContainsFunc(p.prices, func(pr *price) bool { return pr.ref == ref && !pr.archived })
}

// StartCheckout implements supporters.PaymentProvider.
func (p *Provider) StartCheckout(ctx context.Context, req supporters.CheckoutRequest) (supporters.Checkout, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.Checkout{}, err
	}
	if req.Key != "" {
		if ref, ok := p.checkoutKeys[req.Key]; ok {
			return p.checkoutOf(ref), nil
		}
	}
	if req.LocalID == "" || !p.activePrice(req.Price) {
		return supporters.Checkout{}, fmt.Errorf("%w: a checkout needs our ID and an active price", supporters.ErrPaymentInvalidRequest)
	}
	ref := supporters.CheckoutRef(p.newRef("cs"))
	if p.o.Pages {
		if _, err := p.pageURL(req.SuccessURL, ""); err != nil || !sameOrigin(req.SuccessURL, req.CancelURL) {
			return supporters.Checkout{}, fmt.Errorf("%w: a checkout needs return URLs on one origin", supporters.ErrPaymentInvalidRequest)
		}
		// The page's URL is a bearer token of the checkout, as a hosted checkout's.
		ref = supporters.CheckoutRef("cs_" + randomToken())
	}
	r := req
	p.checkouts[ref] = &checkout{
		result:    supporters.CheckoutResult{Ref: ref, Status: supporters.CheckoutOpen, LocalID: req.LocalID, Customer: req.Customer},
		expiresAt: p.o.Now().Add(time.Hour), req: &r,
	}
	if req.Key != "" {
		p.checkoutKeys[req.Key] = ref
	}
	return p.checkoutOf(ref), nil
}

func (p *Provider) checkoutOf(ref supporters.CheckoutRef) supporters.Checkout {
	c := p.checkouts[ref]
	redirect := "https://" + CheckoutHost + "/c/" + url.PathEscape(string(ref))
	if p.o.Pages && c.req != nil {
		redirect, _ = p.pageURL(c.req.SuccessURL, pageCheckout+url.PathEscape(string(ref))) // StartCheckout checked the URL
	}
	return supporters.Checkout{Ref: ref, RedirectURL: redirect, ExpiresAt: c.expiresAt}
}

// GetCheckout implements supporters.PaymentProvider.
func (p *Provider) GetCheckout(ctx context.Context, ref supporters.CheckoutRef) (supporters.CheckoutResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.CheckoutResult{}, err
	}
	c, ok := p.checkouts[ref]
	if !ok {
		return supporters.CheckoutResult{}, fmt.Errorf("%w: checkout %s", supporters.ErrPaymentNotFound, ref)
	}
	if c.result.Status == supporters.CheckoutOpen && !p.o.Now().Before(c.expiresAt) {
		c.result.Status = supporters.CheckoutExpired
	}
	return c.result, nil
}

// PortalURL implements supporters.PaymentProvider.
func (p *Provider) PortalURL(ctx context.Context, customer supporters.CustomerRef, returnURL string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return "", err
	}
	if customer == "" {
		return "", fmt.Errorf("%w: no customer", supporters.ErrPaymentInvalidRequest)
	}
	if p.o.Pages {
		token := randomToken()
		u, err := p.pageURL(returnURL, pagePortal+token)
		if err != nil {
			return "", err
		}
		now := p.o.Now()
		for k, pt := range p.portals {
			if !now.Before(pt.expiresAt) {
				delete(p.portals, k)
			}
		}
		p.portals[token] = &portal{customer: customer, returnURL: returnURL, expiresAt: p.o.Now().Add(time.Hour)}
		return u, nil
	}
	return "https://" + PortalHost + "/p/" + url.PathEscape(string(customer)) + "?return=" + url.QueryEscape(returnURL), nil
}

func (p *Provider) subIndex(ref supporters.SubscriptionRef) int {
	return slices.IndexFunc(p.subs, func(s supporters.SubscriptionSnapshot) bool { return s.Ref == ref })
}

// command runs a command on a subscription that has not ended (CancelNow also on one that has).
func (p *Provider) command(ctx context.Context, ref supporters.SubscriptionRef, ended bool, f func(*supporters.SubscriptionSnapshot) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return err
	}
	i := p.subIndex(ref)
	if i < 0 {
		return fmt.Errorf("%w: subscription %s", supporters.ErrPaymentNotFound, ref)
	}
	if p.subs[i].Status == supporters.StatusCanceled && !ended {
		return fmt.Errorf("%w: subscription %s has ended", supporters.ErrPaymentInvalidRequest, ref)
	}
	return f(&p.subs[i])
}

// ChangePrice implements supporters.PaymentProvider.
func (p *Provider) ChangePrice(ctx context.Context, sub supporters.SubscriptionRef, to supporters.PriceRef, when supporters.ChangeTiming, _ string) error {
	return p.command(ctx, sub, false, func(s *supporters.SubscriptionSnapshot) error {
		if !p.activePrice(to) {
			return fmt.Errorf("%w: price %s is not active", supporters.ErrPaymentInvalidRequest, to)
		}
		switch when {
		case supporters.ChangeNowProrated:
			s.Price, s.ScheduledPrice = to, ""
		case supporters.ChangeAtPeriodEnd:
			s.ScheduledPrice = to
			if to == s.Price {
				s.ScheduledPrice = ""
			}
		default:
			return fmt.Errorf("%w: timing %q", supporters.ErrPaymentInvalidRequest, when)
		}
		return nil
	})
}

// CancelScheduledChange implements supporters.PaymentProvider.
func (p *Provider) CancelScheduledChange(ctx context.Context, sub supporters.SubscriptionRef, _ string) error {
	return p.command(ctx, sub, false, func(s *supporters.SubscriptionSnapshot) error {
		s.ScheduledPrice = ""
		return nil
	})
}

// CancelAtPeriodEnd implements supporters.PaymentProvider.
func (p *Provider) CancelAtPeriodEnd(ctx context.Context, sub supporters.SubscriptionRef, _ string) error {
	return p.command(ctx, sub, false, func(s *supporters.SubscriptionSnapshot) error {
		s.CancelAtPeriodEnd = true
		return nil
	})
}

// ResumeCancellation implements supporters.PaymentProvider.
func (p *Provider) ResumeCancellation(ctx context.Context, sub supporters.SubscriptionRef, _ string) error {
	return p.command(ctx, sub, false, func(s *supporters.SubscriptionSnapshot) error {
		s.CancelAtPeriodEnd = false
		return nil
	})
}

// CancelNow implements supporters.PaymentProvider.
func (p *Provider) CancelNow(ctx context.Context, sub supporters.SubscriptionRef, _ string) error {
	return p.command(ctx, sub, true, func(s *supporters.SubscriptionSnapshot) error {
		s.Status, s.CancelAtPeriodEnd, s.ScheduledPrice = supporters.StatusCanceled, false, ""
		return nil
	})
}

// Resolve implements supporters.PaymentProvider.
func (p *Provider) Resolve(ctx context.Context, n supporters.Notice) (supporters.Change, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.Change{}, err
	}
	var ch supporters.Change
	if n.Subscription != "" {
		ch.Subscriptions = append(ch.Subscriptions, n.Subscription)
	}
	switch n.Kind {
	case supporters.NoticeSubscription:
		ch.Subscriptions = append(ch.Subscriptions, supporters.SubscriptionRef(n.ObjectID))
	case supporters.NoticeCheckout:
		ch.Checkout = supporters.CheckoutRef(n.ObjectID)
	case supporters.NoticeMoney:
		i := slices.IndexFunc(p.money, func(b booked) bool { return b.object == n.ObjectID })
		if i < 0 {
			return supporters.Change{}, fmt.Errorf("%w: money %s", supporters.ErrPaymentNotFound, n.ObjectID)
		}
		if m := p.money[i].m; m.Final {
			ch.Money = append(ch.Money, p.withParent(m))
			if m.Subscription != "" {
				ch.Subscriptions = append(ch.Subscriptions, m.Subscription)
			}
		}
	}
	return ch, nil
}

// GetSubscription implements supporters.PaymentProvider.
func (p *Provider) GetSubscription(ctx context.Context, ref supporters.SubscriptionRef) (supporters.SubscriptionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.SubscriptionSnapshot{}, err
	}
	i := p.subIndex(ref)
	if i < 0 {
		return supporters.SubscriptionSnapshot{}, fmt.Errorf("%w: subscription %s", supporters.ErrPaymentNotFound, ref)
	}
	return p.subs[i], nil
}

// offset parses a cursor of the lists: "<prefix>:<n>", "" for 0.
func offset(cursor, prefix string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	rest, ok := strings.CutPrefix(cursor, prefix+":")
	n, err := strconv.Atoi(rest)
	if !ok || err != nil || n < 0 {
		return 0, fmt.Errorf("%w: cursor %q", supporters.ErrPaymentInvalidRequest, cursor)
	}
	return n, nil
}

// ListSubscriptions implements supporters.PaymentProvider.
func (p *Provider) ListSubscriptions(ctx context.Context, cursor string) (supporters.SubscriptionPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.SubscriptionPage{}, err
	}
	from, err := offset(cursor, "sub")
	if err != nil {
		return supporters.SubscriptionPage{}, err
	}
	from = min(from, len(p.subs))
	to := min(from+p.o.PageSize, len(p.subs))
	page := supporters.SubscriptionPage{Subscriptions: slices.Clone(p.subs[from:to])}
	if to < len(p.subs) {
		page.Next = "sub:" + strconv.Itoa(to)
	}
	return page, nil
}

// ListChanges implements supporters.PaymentProvider.
func (p *Provider) ListChanges(ctx context.Context, since time.Time, cursor string) (supporters.ChangePage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.ChangePage{}, err
	}
	now := p.o.Now()
	if since.Before(now.Add(-p.o.ChangesWindow)) {
		return supporters.ChangePage{}, fmt.Errorf("%w: since %s", supporters.ErrChangesWindow, since)
	}
	from, err := offset(cursor, "chg")
	if err != nil {
		return supporters.ChangePage{}, err
	}
	var list []supporters.Notice
	for _, n := range p.changes {
		if !n.OccurredAt.Before(since) {
			list = append(list, n)
		}
	}
	slices.SortStableFunc(list, func(a, b supporters.Notice) int { return a.OccurredAt.Compare(b.OccurredAt) })
	from = min(from, len(list))
	to := min(from+p.o.PageSize, len(list))
	page := supporters.ChangePage{Notices: slices.Clone(list[from:to]), Until: now}
	if to < len(list) {
		page.Next = "chg:" + strconv.Itoa(to)
	}
	return page, nil
}

// withParent fills in the ParentGross of a movement from its parent.
func (p *Provider) withParent(m supporters.MoneyMovement) supporters.MoneyMovement {
	if m.ParentID == "" {
		return m
	}
	if i := slices.IndexFunc(p.money, func(b booked) bool { return b.m.ID == m.ParentID }); i >= 0 {
		m.ParentGross = p.money[i].m.Gross
	}
	return m
}

// ListMoney implements supporters.PaymentProvider. The movements are in the order they became
// final; the cursor is the last one read.
func (p *Provider) ListMoney(ctx context.Context, since time.Time, cursor string) (supporters.MoneyPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.MoneyPage{}, err
	}
	after, err := offset(cursor, "txn")
	if err != nil {
		return supporters.MoneyPage{}, err
	}
	var list []booked
	for _, b := range p.money {
		if b.m.Final && b.seq > after && !b.m.OccurredAt.Before(since) {
			list = append(list, b)
		}
	}
	slices.SortFunc(list, func(a, b booked) int { return cmp.Compare(a.seq, b.seq) })
	page := supporters.MoneyPage{Next: cursor, More: len(list) > p.o.PageSize}
	for _, b := range list[:min(len(list), p.o.PageSize)] {
		page.Movements = append(page.Movements, p.withParent(b.m))
		page.Next = "txn:" + strconv.Itoa(b.seq)
	}
	return page, nil
}

// GetMoney implements supporters.PaymentProvider.
func (p *Provider) GetMoney(ctx context.Context, id string) (supporters.MoneyMovement, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.request(ctx); err != nil {
		return supporters.MoneyMovement{}, err
	}
	i := slices.IndexFunc(p.money, func(b booked) bool { return b.m.ID == id && b.m.Final })
	if i < 0 {
		return supporters.MoneyMovement{}, fmt.Errorf("%w: money %s", supporters.ErrPaymentNotFound, id)
	}
	return p.withParent(p.money[i].m), nil
}
