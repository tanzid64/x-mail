package resend

import (
	"context"
	"time"

	"github.com/dark/x-mail/api/internal/providers"
)

type domainRecord struct {
	Record   string `json:"record"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      string `json:"ttl"`
	Status   string `json:"status"`
	Value    string `json:"value"`
	Priority *int   `json:"priority"`
}

type domainPayload struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Status    string         `json:"status"`
	Region    string         `json:"region"`
	CreatedAt string         `json:"created_at"`
	Records   []domainRecord `json:"records"`
}

type listDomainsResponse struct {
	Data []domainPayload `json:"data"`
}

func (c *Client) ListDomains(ctx context.Context) ([]providers.Domain, error) {
	var resp listDomainsResponse
	if err := c.do(ctx, "GET", "/domains", nil, nil, &resp); err != nil {
		return nil, err
	}

	domains := make([]providers.Domain, 0, len(resp.Data))
	for _, payload := range resp.Data {
		domains = append(domains, payload.toDomain())
	}
	return domains, nil
}

func (c *Client) GetDomain(ctx context.Context, id string) (*providers.Domain, error) {
	var payload domainPayload
	if err := c.do(ctx, "GET", "/domains/"+id, nil, nil, &payload); err != nil {
		return nil, err
	}
	domain := payload.toDomain()
	return &domain, nil
}

func (p domainPayload) toDomain() providers.Domain {
	domain := providers.Domain{
		ID:     p.ID,
		Name:   p.Name,
		Status: mapDomainStatus(p.Status),
		Region: p.Region,
	}

	if created, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
		domain.CreatedAt = &created
	}

	domain.Records = make([]providers.DNSRecord, 0, len(p.Records))
	for _, r := range p.Records {
		domain.Records = append(domain.Records, providers.DNSRecord{
			Type:     r.Type,
			Name:     r.Name,
			Value:    r.Value,
			TTL:      r.TTL,
			Priority: r.Priority,
			Status:   r.Status,
		})
	}
	return domain
}

// mapDomainStatus collapses Resend's lifecycle onto the three states the
// application cares about: still waiting on DNS, usable, or hopeless.
func mapDomainStatus(status string) providers.DomainStatus {
	switch status {
	case "verified":
		return providers.DomainVerified
	case "failed", "temporary_failure":
		return providers.DomainFailed
	default:
		// "not_started", "pending" and anything new default to pending.
		return providers.DomainPending
	}
}
