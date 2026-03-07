# ZIM Compatibility Issues

Notes on known gaps between zimserve and the Kiwix ecosystem, discovered by inspecting real ZIM files.

---

## Background

ZIM files produced by [mwoffliner](https://github.com/openzim/mwoffliner) (the Kiwix scraper for MediaWiki sites) are designed to be served by the **Kiwix app** or **Kiwix-serve**, both of which wrap ZIM content with their own application shell. That shell provides navigation, search, and browse features that are _not_ part of the ZIM HTML itself.

When serving ZIM files with a generic HTTP server like zimserve, those features are absent.

---

## Investigated ZIM

`wiktionary_en_simple_all_nopic_2026-01.zim` — Simple English Wiktionary, 52,658 entries, Vector 2022 skin.

---

## Issue 1: A–Z Browse Navigation — links not present in ZIM HTML

### Symptom
The Main Page renders a "Browse: A a B b C c … Z z" table. The letters appear styled as if they should be links, but clicking them does nothing.

### Root cause
The original MediaWiki page used `[[Special:Allpages/A|A]]` wikilinks. mwoffliner strips all `Special:` page links because those pages don't exist in a ZIM. The result is plain `<td>A</td>` cells with no `href`. A remnant comment in the HTML confirms this:

```html
<!--[[Special:Allpages/Ά|specials]]-->
```

The Kiwix app restores browse-by-letter using its own UI chrome, backed by the binary title index embedded in the ZIM at `X/listing/titleOrdered/v1` (MIME: `application/octet-stream+zimlisting`, ~210 KB for this ZIM).

### Status
**Not a zimserve bug.** The links were intentionally removed from the ZIM content. Kiwix's app shell silently fills the gap.

### Possible fix
zimserve could implement a `/{slug}/-/search?prefix=A` browse endpoint and inject a navigation bar into HTML responses at serve time. The title prefix search is already achievable with our existing `EntriesByTitle` iterator (no Xapian needed). This is tracked as a potential Phase 5 enhancement.

---

## Issue 2: Search Box — not rendered, would 404 if submitted

### Symptom
The Main Page includes text "Use the search box provided to find words in Wiktionary" but no visible input field appears.

### Root cause — part 1: Vue component not rendered
The search input is a [Codex](https://doc.wikimedia.org/codex/) Vue.js component:

```html
<form action="/wiki/Special:Search" ...>
  <div class="cdx-text-input"></div>  <!-- rendered empty without MW JS -->
</form>
```

The `<input>` element is injected at runtime by MediaWiki's JS bundle. This ZIM has no `_mw_/*.js` files — mwoffliner does not bundle MediaWiki's ResourceLoader JS. Without the JS, the div remains empty and the search box is invisible.

### Root cause — part 2: search endpoint not implemented
Even if the input rendered, the form posts to `/wiki/Special:Search` (an absolute root-relative URL). In zimserve this routes as slug `wiki`, path `Special:Search` → 404.

Kiwix-serve handles this endpoint using two Xapian databases embedded in the ZIM:
- `X/fulltext/xapian` (MIME: `application/octet-stream+xapian`, ~6 MB) — full-text index
- `X/title/xapian` (MIME: `application/octet-stream+xapian`, ~5.9 MB) — title index

These are binary [Xapian](https://xapian.org/) databases. Querying them requires libxapian, which has no pure-Go implementation.

### Status
**Mixed.** The invisible input is a ZIM design assumption (MW JS expected). The missing endpoint is a zimserve gap.

### Possible fix
- **Title prefix search** (good enough for a dictionary): implement `/{slug}/wiki/Special:Search?search=term` in zimserve using `Archive.EntriesByTitle`. No Xapian needed. Returns an HTML results page linking to matching entries.
- **Full-text search**: requires either libxapian (CGo) or reimplementing Xapian's on-disk format. Deferred per PLAN.md.

---

## Issue 3: Inline search form uses absolute URL path

### Symptom
The inline `<form action="/wiki/Special:Search">` on the Main Page uses a root-relative URL. In zimserve, `wiki` is interpreted as a slug → 404.

### Root cause
mwoffliner preserves the original MediaWiki URL structure inside the ZIM. Kiwix-serve mounts ZIM content at `/` so `/wiki/Special:Search` works. zimserve mounts at `/{slug}/`, so it doesn't.

### Status
**zimserve routing gap.** Even a partial fix (title search endpoint) would need to handle both `/wiki/Special:Search` and `/{slug}/wiki/Special:Search` patterns, or zimserve needs to rewrite form actions in HTML responses.

---

## Summary Table

| Feature | In ZIM HTML | Kiwix provides | zimserve status |
|---|---|---|---|
| A–Z browse links | No (stripped) | Yes (app shell + `X/listing`) | Missing — no app shell |
| Search input field | Stub div only | Yes (Vue component via MW JS) | Not renderable |
| Search submission | `/wiki/Special:Search` (absolute) | Yes (Xapian backend) | 404 — no endpoint |
| Word entry links | Yes (relative hrefs) | Yes | **Works** |
| CSS / images / fonts | Yes (in `C/_mw_/`, `C/_res_/`) | Yes | **Works** |
| Redirects | Yes (ZIM redirect entries) | Yes | **Works** |

---

## Relevant ZIM Entries

| Path | MIME | Purpose |
|---|---|---|
| `X/listing/titleOrdered/v1` | `application/octet-stream+zimlisting` | Sorted title list for browse-by-letter |
| `X/fulltext/xapian` | `application/octet-stream+xapian` | Full-text search index |
| `X/title/xapian` | `application/octet-stream+xapian` | Title search index |
| `C/_mw_/skins.vector.styles.css` | `text/css` | Vector 2022 skin CSS |
| `C/_res_/script.js` | `application/javascript` | Section collapse + WebP polyfill |
