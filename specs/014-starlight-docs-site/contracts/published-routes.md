# Contract: Published Routes

**Feature**: `014-starlight-docs-site`

The site's external contract to readers and crawlers: the set of URLs that MUST
return successfully under the project subpath, and the link-integrity guarantee.
All URLs are rooted at `base: '/gh-aw-fleet'` on `site:
'https://rshade.github.io'`.

## Route table (MUST return HTTP 200)

| Path | Page | FR / SC | ≤2 clicks from home? |
|---|---|---|---|
| `/gh-aw-fleet/` | Splash landing | FR-001, SC-001 | — (home) |
| `/gh-aw-fleet/install/` | Install & quickstart | FR-002(a), SC-002 | yes |
| `/gh-aw-fleet/configuration/` | `fleet.json` reference | FR-002(b), SC-002 | yes |
| `/gh-aw-fleet/reconcile/` | deploy / sync / upgrade | FR-002(c), SC-002 | yes |
| `/gh-aw-fleet/consumption/` | FinOps / consumption | FR-002(d) | yes |
| `/gh-aw-fleet/roadmap/` | Roadmap | FR-002(e) | yes |

Generated/auxiliary routes (MUST also resolve):

| Path | Source |
|---|---|
| `/gh-aw-fleet/sitemap-index.xml` | `@astrojs/sitemap` |
| `/gh-aw-fleet/robots.txt` | `docs/public/robots.txt` |
| `/gh-aw-fleet/llms.txt` | `docs/public/llms.txt` |
| `/gh-aw-fleet/favicon.svg` | `docs/public/favicon.svg` |
| `/gh-aw-fleet/pagefind/*` | Starlight default search index |

## Invariants

- **C1 (subpath correctness, FR-003 / SC-006)**: Every internal link and asset
  reference resolves under `/gh-aw-fleet`. No link assumes the domain root.
  *Enforced at build* by `starlight-links-validator` (fails `astro build` on any
  broken internal link). *Verified post-deploy* by a full-site link check (SC-006:
  zero internal 404s).
- **C2 (navigation depth, SC-002)**: Install, configuration, and reconcile are
  each reachable from the landing page in ≤2 clicks (sidebar + landing hero/cards).
- **C3 (content completeness, FR-002 / SC-007)**: All six pages above exist and
  are populated from the named seed sources; the migrated `consumption` page
  carries the former `docs/finops.md` content.
- **C4 (sitemap base, Decision 5)**: The sitemap index and the `robots.txt`
  `Sitemap:` directive both reference the `/gh-aw-fleet`-prefixed absolute URL.

## Acceptance check (maps to SC-001, SC-006)

```
for path in / /install/ /configuration/ /reconcile/ /consumption/ /roadmap/ \
            /sitemap-index.xml /robots.txt /llms.txt ; do
  curl -fsS -o /dev/null "https://rshade.github.io/gh-aw-fleet${path}" \
    && echo "200  ${path}" || echo "FAIL ${path}"
done
```

A full-site crawler (or the build-time `starlight-links-validator` pass) confirms
zero internal links 404 (SC-006).
