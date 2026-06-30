# niche-finder

Finds **digital / printable product niches** (printables, cookbooks, planners,
wall art, SVGs, templates — no physical fulfillment) and tells you **how crowded
each one is**. You give it high-level **seed categories**; a small Go service uses
the home [LLM cluster](../../infra/llm-cluster/) (Qwen, via HAProxy) to expand each
into long-tail **SEO keywords**, validates them with guided web research,
measures competition, and ranks them by opportunity on a one-page dashboard you
manage.

```
seeds → Qwen expands to SEO keywords → guided research (Reddit search + critic)
      → saturation meter (real count, LLM fallback) → manage on the board
```

You never type keywords — you set categories like `Cookbooks` and the AI derives
the niche keywords. This is the first of several **single-concern** apps that
replace the old all-in-one `ecomm-pipeline`.

## Run

```bash
cp .env.example .env          # defaults point at the cluster
./edge up niche-finder --build
# or, from this dir:  docker compose up --build
```

Then open **http://<host>:8820** and click **Find niches**. The only hard
dependency is the LLM cluster; everything else degrades gracefully.

> The default model is `qwen2.5:14b` — make sure a node serves it
> (`./edge model set qwen2.5:14b`, or point `LLM_MODEL` at a Qwen tag you have).

## How it works

A discovery run is a guided, multi-phase research loop (the LLM proposes what to
investigate from your seeds + live leads — you supply no keywords):

| Phase | What it does |
|-------|--------------|
| **Leads** | Pluggable `Source` adapters fetch raw text: **Reddit** hot posts (public `*.json`), **Google Trends** daily queries (RSS), **Etsy** (official API, opt-in with a key). |
| **Propose themes** | Qwen expands your **seed categories** into ~12 themes, each with long-tail SEO search terms (applying keyword rules: buyer intent, modifiers like "printable/template/svg", specificity). |
| **Deepen** | Each term is run through **Reddit search** to gather real demand evidence. |
| **Critique** | A skeptic pass finds gaps and proposes fresh follow-up terms → a second deepen round. |
| **Synthesize** | Per theme, Qwen distills concrete digital-product niches grounded in the evidence (name, audience, product, rationale + a list of long/short-tail keywords). |
| **Score opportunity** | Each keyword gets an **opportunity score** (0–100) — see below. |

### Opportunity score (per keyword)
A composite, not just a listing count — `opportunity` rewards **high demand, low
competition, strong intent**:

| Signal | How it's measured | Cost |
|--------|-------------------|------|
| **Demand** | Google autocomplete depth (how many real-search completions the phrase/root yields) | free, no key, always on |
| **Competition** | Etsy listing **count + incumbent strength** (median favorites of the top listings) when keyed; eBay count if you enable it; LLM estimate as last resort | free; needs the Etsy key for the real signal |
| **Intent** | phrase heuristics — long-tail + buyer modifiers (`printable`, `template`, `svg`, …) | free, no key, always on |

`opportunity = 0.4·demand + 0.4·(100−competition) + 0.2·intent`, reweighted when a
signal is missing. A keyword is **high confidence** only when both demand and a
*measured* (Etsy) competition exist — otherwise it's flagged **low** (e.g. before
your Etsy key lands, demand+intent carry the score). The board sorts favorites
first, then by opportunity; each niche's headline is its best keyword.

Each niche is an **item niche name** with a **list of SEO keywords** (long- and
short-tail), and **every keyword is scored individually** so you can see which
exact phrase is the open lane. A niche's headline opportunity is its best keyword.

### Managing it (the dashboard)
- **Seed categories** — add/remove categories; run discovery for one category or all.
- **Add your own keyword** — track a specific keyword; the AI still profiles it
  (audience/product/rationale) and expands it into related phrases, all scored — it
  doesn't bypass the AI.
- **Curate** — favorite/shortlist, re-score saturation on demand (competition
  drifts), archive, or delete each niche.

### API
The board is also a read API for other tools:

```
GET /api/niches              # JSON: [{name, category, audience, product, opportunity,
                             #         keywords:[{phrase, type, opportunity, demand,
                             #                    competition, intent, confidence, ...}]}]
GET /api/niches?format=text  # plain text report (also /api/niches.txt)
```

Filters: `?category=<seed>` (match a seed category), `?show=all` (include archived).

Adding a source is another `Source` in [`internal/source`](internal/source/); the
research phases live in [`internal/niche`](internal/niche/research.go); saturation
backends are cases in [`internal/saturation`](internal/saturation/saturation.go).

## Configuration (`.env`)

| Var | Default | Meaning |
|-----|---------|---------|
| `LLM_BASE_URL` | `http://host.docker.internal:11434` | cluster endpoint; `edge up` injects the master from `fleet.json` |
| `LLM_MODEL` | `qwen2.5:14b` | Qwen tag served by the cluster |
| `SEED_CATEGORIES` | *(empty)* | initial seed categories (e.g. `Cookbooks,Printable planners`); bootstraps the DB once, then the UI owns the list |
| `NICHE_SOURCES` | `reddit:somethingimade,Etsy,crafts;gtrends:` | enabled lead-source adapters + args |
| `MINE_LIMIT` | `25` | items pulled per source |
| `ETSY_API_KEY` | *(empty)* | Etsy Open API v3 keystring; set ⇒ Etsy mining + measured saturation |
| `SATURATION_MARKETS` | `etsy,ebay` | measured-saturation try order (LLM estimate as final fallback) |
| `NICHE_PORT` | `8820` | host port (container always listens on 8820) |

## Layout

```
cmd/niche-finder/   main: config → store → finder + web server, graceful shutdown
internal/
  config/           env-driven configuration
  llm/              langchaingo Ollama wrapper (text + JSON helpers)
  source/           Source interface + reddit (hot+search) / gtrends / etsy + registry
  etsy/             tiny Etsy Open API v3 client (shared by source + saturation)
  niche/            research engine (propose/critique/synthesize) + Finder orchestration
  saturation/       Scorer: keyword → 0-100 meter (etsy/ebay count, LLM fallback)
  model/            Niche domain type (+ management state)
  store/            bbolt-backed Niche repo + managed seed list (pure Go, no CGO)
  web/              management dashboard (embedded html/template)
```

## Develop / test (Go installed locally)

```bash
go test ./...                              # parsing + scoring logic, no network
LLM_BASE_URL=http://<master>:11434 \
  go run ./cmd/niche-finder                # DATA_DIR defaults to /data
```

State (the niche DB) lives in `DATA_DIR` (`/data` in the container, backed by the
`niche-data` volume so it survives rebuilds).
