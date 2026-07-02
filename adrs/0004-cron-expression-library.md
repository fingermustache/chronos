# 4. Cron Expression Library

Date: 2026-07-02

## Status

Accepted

## Context

Chronos supports `schedule_type: cron` with an `expression` field in `schedule_config`.
The API gateway must validate that a submitted cron expression is syntactically correct before persisting it.
Writing a cron parser from scratch is error-prone and outside the project's scope.

## Alternatives Considered

### `github.com/robfig/cron/v3` ✓ chosen

The de facto standard Go cron library.
Exposes a configurable `Parser` that returns a parsed `Schedule` or a descriptive error.
Zero transitive dependencies.
Stable API since 2019 with active maintenance.
Supports standard five-field format and optional seconds/years fields via parser flags.

### `github.com/adhocore/gronx`

A lightweight cron expression validator with no scheduler attached — closer in scope to what Chronos needs.
Supports standard five-field, six-field (with seconds), and seven-field (with years) expressions, plus `@` shorthand descriptors (`@daily`, etc.).
Zero transitive dependencies and marginally smaller footprint than `robfig/cron/v3`.
Less adoption than `robfig/cron/v3` (roughly 10× fewer GitHub stars at time of writing), so less battle-tested in production.

### `github.com/gorhill/cronexpr`

A pure cron expression parser with no scheduler.
Supports extended syntax including `L` (last), `W` (nearest weekday), and `#` (nth weekday) modifiers beyond what most UNIX cron implementations accept.
The richer syntax is a liability here: Chronos only needs to validate expressions, and accepting non-standard modifiers would create a mismatch between what the API accepts and what the scheduler engine will eventually support.
The library has been effectively unmaintained since 2016.

### Regex-based structural check

A hand-written regular expression can confirm that an expression has five whitespace-separated fields each containing only valid characters.
This is zero-dependency and trivially fast, but it cannot catch semantic errors such as `99 * * * *` (minute out of range) or `0 0 31 2 *` (February 31).
Catching only syntax while silently accepting invalid schedules would push errors to runtime rather than the API boundary.

## Decision

Use `github.com/robfig/cron/v3` solely for expression parsing and validation.
The full scheduler (`cron.Cron`) is not used — Chronos builds its own scheduling engine.

`gronx` was the closest alternative but `robfig/cron/v3`'s significantly wider adoption means more community validation of edge cases.
`cronexpr`'s extended modifier syntax and unmaintained status ruled it out.
The regex approach was rejected because it cannot catch semantic errors.

## Consequences

- One new direct dependency in `src/go.mod`.
- Only the `cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expr)` surface is used.
- The standard five-field format is enforced (`* * * * *` = minute hour dom month dow).
  Second-field expressions are not accepted; if needed, the parser can be reconfigured.
- Validation is cheap — pure parsing, no goroutines spun up.
