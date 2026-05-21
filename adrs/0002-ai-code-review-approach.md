# 2. Use Manual AI Code Review

Date: 2026-05-21

## Status

Accepted

## Context

Evaluated automated AI PR review solutions for personal projects to:
- Improve code quality
- Demonstrate DevOps/AI integration skills
- Minimize costs (target: <$2/month or free)

### Options Evaluated

| Solution | Privacy Model | Cost Model | Compliance Concerns |
|----------|---------------|------------|---------------------|
| **CodeRabbit** | Third-party SaaS with full repo access | Free (public repos), $12/month (private) | Private code sent to external servers; third-party OAuth access |
| **Gemini API** | Google-hosted API | Free tier available | Terms restrict to "professional or business purposes, not for consumer use" |
| **Claude API** | Anthropic-hosted API | $5 free credit, then pay-per-use | Free credit expires; ongoing costs for personal projects |
| **OpenAI API** | OpenAI-hosted API | $5 free credit (3-month expiry), then pay-per-use | Similar cost concerns as Claude |
| **Groq/Open Models** | Various providers | Free tier available | Evaluated but not selected |

## Decision

Use conversational AI (chat interface) for manual code review assistance instead of automated PR review integration.

### Rationale

**CodeRabbit rejected:** Privacy concerns with granting third-party OAuth access to private repositories. Code and intellectual property would be sent to external servers outside of direct control.

**Gemini API rejected:** Google's terms of service explicitly state "for professional or business purposes, not for consumer use." Individual/personal use falls into gray area with compliance uncertainty.

**Claude/OpenAI APIs rejected:** Cost concerns for ongoing usage. Free credits expire, creating potential recurring costs incompatible with personal project budget constraints.

## Consequences

### Positive

- **Zero cost** - No API fees or subscriptions
- **Maximum privacy** - Code remains local; only specific excerpts shared when needed
- **Full control** - Manual curation of what code is reviewed and when
- **No compliance risk** - No ambiguous terms of service for personal use
- **Flexibility** - Can still demonstrate AI-assisted development

### Negative

- **No automation** - Reviews require manual effort (copy/paste code to chat)
- **No CI/CD integration** - Cannot showcase automated pipeline setup
- **Less resume impact** - Missing "implemented automated CI/CD with LLM integration" talking point
- **Manual workflow** - No automatic PR comments; must manually incorporate feedback

### Mitigation Strategies

- Document the evaluation process (this ADR) to show technical decision-making skills
- Use chat assistance during development to still benefit from AI code review
- Focus resume on other automation/DevOps achievements
- Revisit automation when working on professional/commercial projects with clear business context

## Alternatives Considered

- Self-hosted PR-Agent with GitHub Actions (still requires LLM API, inherits same cost/compliance concerns)
- Manual diff extraction script + chat (added complexity for minimal benefit over direct chat use)

## Notes

- This decision applies to **personal/individual projects only**
- For professional or team projects with clear business context and budget, automated solutions may be more appropriate

