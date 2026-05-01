# DNS and Email Setup for allisonrosememorialfund.org

## Overview

The domain uses two email services:

| Service | Purpose |
|---------|---------|
| **ImprovMX** | Receives inbound mail and forwards to Gmail |
| **Resend** | Programmatic/transactional outbound sending |

Resend is built on top of AWS SES, so some DNS records reference `amazonaws.com` domains — these are Resend's infrastructure, not a separately managed service. Inbound mail lands in `ianrose14@gmail.com` via ImprovMX forwarding. Outbound "send as" aliases are configured in Gmail settings.

---

## DNS Records

### MX Records

| Host | Value | Priority |
|------|-------|----------|
| `@` | `mx1.improvmx.com` | 10 |
| `@` | `mx2.improvmx.com` | 20 |
| `send` | `feedback-smtp.us-east-1.amazonaws.com` | 10 |

The `@` records route inbound mail through ImprovMX. The `send` subdomain MX is used by Amazon SES for bounce/feedback handling.

### TXT Records (SPF, DMARC, verification)

| Host | Value | Purpose |
|------|-------|---------|
| `@` | `v=spf1 include:spf.efwd.registrar-servers.com include:spf.improvmx.com ~all` | SPF for outbound sending |
| `_dmarc` | `v=DMARC1; p=none;` | DMARC policy |
| `send` | `v=spf1 include:amazonses.com ~all` | SPF for SES sending subdomain |
| `resend._domainkey` | *(DKIM public key)* | DKIM for Resend |
| `_github-pages-...` | *(verification token)* | GitHub Pages domain verification |

### Host Records (A / AAAA / CNAME)

| Type | Host | Value | Purpose |
|------|------|-------|---------|
| A | `@` | `185.199.108/109/110/111.153` | GitHub Pages (website) |
| AAAA | `@` | `2606:50c0:8000::153` | GitHub Pages IPv6 |
| CNAME | `www` | `ianrose14.github.io.` | GitHub Pages www redirect |

---

## Inbound Email Flow

```
Sender → MX lookup → mx1.improvmx.com → forwarded to ianrose14@gmail.com
```

ImprovMX forwarding rules are managed at [improvmx.com](https://improvmx.com).

## Outbound Email (Gmail "Send mail as")

Gmail is configured to send from the following aliases via `smtp.gmail.com` on port 587:
- `applications@allisonrosememorialfund.org`
- `ianrose@allisonrosememorialfund.org`

These are configured under Gmail Settings → Accounts and Import → "Send mail as."

### Automated emails from `Resend`

A [github cronjob](https://github.com/ianrose14/scholarshipfund/blob/main/.github/workflows/cadence-reminder.yml) occasionally sends automated
reminder emails via the `check_cadence.py` script.  This uses the [Resend](https://resend.com/) REST API to do so.

---

## Troubleshooting Notes

- **If inbound mail stops working:** Check that the `@` MX records still point to ImprovMX. In the past, the MX record was accidentally deleted, causing senders to fall back to the domain's A record (GitHub Pages IPs), which does not accept SMTP connections.
- **Namecheap mail settings mode** must remain set to **"Custom MX"** — switching to "Email Forwarding" mode would remove the ability to add custom MX records and break the Resend/SES setup.
- **DNS propagation** after any MX change typically takes 15–60 minutes via Namecheap's nameservers.
- To verify current DNS state: `python3 -c "import dns.resolver; [print(r) for r in dns.resolver.resolve('allisonrosememorialfund.org', 'MX')]"`
