# Changelog

## v0.4.0 — 2026-10-02

- Report the installed module version in the CLI, while preserving build-time overrides.
- Reject placeholder admin credentials, generate independent scaffold secrets, and enforce login throttling.
- Protect edits with revision checks, retain redirects after renames, and validate request and image limits.
- Use installation-scoped analytics identifiers and explicit page-view events; shut down requests and workers cleanly.
- Accept standard Markdown images, copy real dimensions, and lazy-load later article images.
- Apply only the latest navigation response, commit history after success, restore Back navigation, and synchronize page metadata.
- Bind generated views to runtime site settings and honor analytics environment variables.
- Keep editor errors in a full page, redirect after saves, and handle image errors and expired sessions.
- Add summary pagination, indexed post lookup, bounded related posts, and bounded article-body caching.
- Propagate request contexts, use consistent analytics snapshots, coalesce reports, and bound referrer output.
- Add response validators, compress text assets, and reuse rendered Markdown and XML.
- Limit concurrent uploads and use bounded file-backed body parsing.
- Exclude admin analytics, count visible-tab duration, label recent views and beacon bots accurately, and pause hidden-tab polling.
- Normalize legacy referrers by primary key, batch retention cleanup, and verify migration rollback.
- Align tag/date semantics, centralize canonical post paths, and provide missing-view fallbacks.
- Pin generators and frontend tools; add race, browser, migration, and benchmark checks.

See README for integration changes, cache limits, and measured performance samples.
