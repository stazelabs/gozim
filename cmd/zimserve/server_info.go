package main

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (lib *library) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(lib.startTime)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var totEntries, totCacheNow, totCacheCap int
	var totCacheBytes, totHits, totMisses int64
	var zimRows []serverInfoZIMRow
	for _, slug := range lib.slugs {
		ze := lib.archives[slug]
		cs := ze.archive.CacheStats()
		entries := int(ze.archive.EntryCount())
		var cacheVal float64
		var cacheStr, hitRate string
		if total := cs.Hits + cs.Misses; total > 0 {
			cacheVal = float64(cs.Hits) / float64(total) * 100
			cacheStr = fmt.Sprintf("%d/%d", cs.Size, cs.Capacity)
			hitRate = fmt.Sprintf("%.1f%% (%d hits, %d misses)", cacheVal, cs.Hits, cs.Misses)
		} else {
			cacheStr = fmt.Sprintf("%d/%d", cs.Size, cs.Capacity)
			hitRate = "—"
		}
		totEntries += entries
		totCacheNow += cs.Size
		totCacheCap += cs.Capacity
		totCacheBytes += cs.Bytes
		totHits += cs.Hits
		totMisses += cs.Misses
		zimRows = append(zimRows, serverInfoZIMRow{
			Slug:          slug,
			Title:         ze.title,
			EntryCount:    entries,
			CacheNow:      cs.Size,
			CacheCap:      cs.Capacity,
			CacheVal:      cacheVal,
			CacheStr:      cacheStr,
			CacheHitRate:  hitRate,
			CacheBytes:    formatBytesShort(cs.Bytes),
			CacheBytesRaw: cs.Bytes,
		})
	}

	totHitRateStr := "—"
	if t := totHits + totMisses; t > 0 {
		totHitRateStr = fmt.Sprintf("%.1f%% (%d hits, %d misses)", float64(totHits)/float64(t)*100, totHits, totMisses)
	}

	data := serverInfoData{
		NoInfo:        lib.noInfo,
		Uptime:        formatUptime(uptime),
		Started:       lib.startTime.Format("2006-01-02 15:04:05 MST"),
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		Goroutines:    runtime.NumGoroutine(),
		HeapInuse:     formatBytesShort(int64(ms.HeapInuse)),
		HeapIdle:      formatBytesShort(int64(ms.HeapIdle)),
		Sys:           formatBytesShort(int64(ms.Sys)),
		NumGC:         ms.NumGC,
		ZIMs:          zimRows,
		ZIMCount:      len(lib.slugs),
		TotEntries:    totEntries,
		TotCacheNow:   totCacheNow,
		TotCacheCap:   totCacheCap,
		TotCacheBytes: formatBytesShort(totCacheBytes),
		TotHitRate:    totHitRateStr,
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		data.BuildInfo = true
		data.BuildRows = append(data.BuildRows, serverInfoBuildRow{Label: "Main Module", Value: bi.Main.Path})
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			data.BuildRows = append(data.BuildRows, serverInfoBuildRow{Label: "Module Version", Value: bi.Main.Version})
		}
		var settings []string
		for _, s := range bi.Settings {
			if s.Value != "" {
				settings = append(settings, s.Key+"="+s.Value)
			}
		}
		if len(settings) > 0 {
			data.BuildRows = append(data.BuildRows, serverInfoBuildRow{Label: "Settings", Value: strings.Join(settings, " · ")})
		}
		if len(bi.Deps) > 0 {
			deps := make([]*debug.Module, len(bi.Deps))
			copy(deps, bi.Deps)
			sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
			for _, d := range deps {
				replaceStr := ""
				if d.Replace != nil {
					replaceStr = d.Replace.Path
					if d.Replace.Version != "" {
						replaceStr += " " + d.Replace.Version
					}
				}
				data.Deps = append(data.Deps, serverInfoDep{
					Path:    d.Path,
					Version: d.Version,
					Replace: replaceStr,
				})
			}
		}
	}

	renderWith(w, tmplServerInfo, data)
}

// formatUptime formats a duration as "Xd Yh Zm Ws", omitting leading zero units.
func formatUptime(d time.Duration) string {
	d = d.Truncate(time.Second)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, secs)
	case hours > 0:
		return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
	case mins > 0:
		return fmt.Sprintf("%dm %ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// formatBytesShort formats b as a short human-readable string without the raw byte count.
func formatBytesShort(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(1<<10))
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}
